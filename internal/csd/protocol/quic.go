package csdprotocol

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"sync/atomic"
	"time"

	"capper/internal/cert"
	"capper/internal/csd"
	csdserver "capper/internal/csd/server"

	"github.com/google/uuid"
	quic "github.com/quic-go/quic-go"
)

const CSDPPort = 7473

// QUICListener accepts CSDP connections and dispatches them to a Server.
type QUICListener struct {
	listener *quic.Listener
	server   *csdserver.Server
	// tlsCfg holds the active server TLS config. It is swapped atomically by
	// SetTLSConfig so that certificate rotation takes effect on new connections
	// without restarting the listener.
	tlsCfg atomic.Pointer[tls.Config]
}

// NewQUICListener creates a QUIC listener bound to addr using tlsCfg.
// The listener serves the current config via GetConfigForClient, so SetTLSConfig
// can rotate the certificate on live listeners without a restart.
func NewQUICListener(addr string, tlsCfg *tls.Config, srv *csdserver.Server) (*QUICListener, error) {
	ql := &QUICListener{server: srv}
	ql.tlsCfg.Store(tlsCfg)
	// Wrap the config so each incoming handshake reads the latest stored config.
	dynamic := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return ql.tlsCfg.Load(), nil
		},
	}
	l, err := quic.ListenAddr(addr, dynamic, &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("csdp: listen %s: %w", addr, err)
	}
	ql.listener = l
	return ql, nil
}

// SetTLSConfig atomically swaps the active server TLS config. New connections
// use the new config; existing connections are unaffected.
func (l *QUICListener) SetTLSConfig(cfg *tls.Config) {
	l.tlsCfg.Store(cfg)
}

// StartCertRotation runs a loop that re-issues the node's CSD certificate via
// certMgr when it is within 30 days of expiry, swapping it into the live
// listener. It blocks until ctx is cancelled and is intended to run in a
// goroutine. checkInterval controls how often expiry is checked (24h is typical).
func (l *QUICListener) StartCertRotation(ctx context.Context, certMgr *cert.Manager, nodeID string, checkInterval time.Duration) {
	if checkInterval <= 0 {
		checkInterval = 24 * time.Hour
	}
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			expiry, err := certMgr.GetExpiry(nodeID)
			if err != nil || time.Until(expiry) < 30*24*time.Hour {
				newCfg, terr := CSDServerTLS(certMgr, nodeID)
				if terr != nil {
					log.Printf("csdp: cert rotation for %s failed: %v", nodeID, terr)
					continue
				}
				l.SetTLSConfig(newCfg)
				log.Printf("csdp: rotated TLS certificate for node %s", nodeID)
			}
		}
	}
}

// Serve accepts connections until ctx is cancelled.
func (l *QUICListener) Serve(ctx context.Context) error {
	for {
		conn, err := l.listener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go l.handleConn(ctx, conn)
	}
}

func (l *QUICListener) Close() error {
	return l.listener.Close()
}

func (l *QUICListener) handleConn(ctx context.Context, conn *quic.Conn) {
	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go l.handleStream(ctx, stream)
	}
}

func (l *QUICListener) handleStream(ctx context.Context, stream *quic.Stream) {
	defer stream.Close()
	msgType, payload, err := ReadFrame(stream)
	if err != nil {
		return
	}
	if err := l.dispatch(ctx, stream, msgType, payload); err != nil {
		_ = WriteError(stream, ErrCodeInternal, err.Error(), 0)
	}
}

func (l *QUICListener) dispatch(ctx context.Context, w io.Writer, msgType byte, payload []byte) error {
	srv := l.server
	switch msgType {
	case MsgHello:
		var msg HelloMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		v, err := srv.Volumes.Get(ctx, msg.VolumeID, "")
		if err != nil {
			return WriteError(w, ErrCodeNotFound, "volume not found", 0)
		}
		sessionID := uuid.New().String()
		return WriteFrame(w, MsgHelloOK, HelloOKMsg{
			SessionID:     sessionID,
			Epoch:         v.Epoch,
			VolumeMode:    v.Mode,
			MaxReadBytes:  4 * 1024 * 1024,
			MaxWriteBytes: 4 * 1024 * 1024,
			LeaseTTLSecs:  15,
		})

	case MsgGetattr:
		var msg GetattrMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		inode, err := srv.Metadata.Getattr(ctx, msg.InodeID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		return WriteFrame(w, MsgGetattrResp, toInodeMsg(inode))

	case MsgLookup:
		var msg LookupMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		inode, err := srv.Metadata.Lookup(ctx, "", msg.ParentID, msg.Name)
		if err == csd.ErrNotFound {
			return WriteError(w, ErrCodeNotFound, "not found", 0)
		}
		if err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgLookupResp, toInodeMsg(inode))

	case MsgReaddir:
		var msg ReaddirMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		// volumeID is not in the message; look up the parent to get it.
		parent, err := srv.Metadata.Getattr(ctx, msg.DirID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		children, err := srv.Metadata.Readdir(ctx, parent.VolumeID, msg.DirID)
		if err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		var entries []InodeMsg
		for _, c := range children {
			entries = append(entries, toInodeMsg(c))
		}
		return WriteFrame(w, MsgReaddirResp, ReaddirRespMsg{Entries: entries})

	case MsgCreate:
		var msg CreateMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		parent, err := srv.Metadata.Getattr(ctx, msg.ParentID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		inode, err := srv.Metadata.Create(ctx, csdserver.CreateReq{
			VolumeID: parent.VolumeID, ParentID: msg.ParentID,
			Name: msg.Name, ModeBits: msg.Mode,
		})
		if err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgCreateResp, toInodeMsg(inode))

	case MsgMkdir:
		var msg MkdirMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		parent, err := srv.Metadata.Getattr(ctx, msg.ParentID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		inode, err := srv.Metadata.Mkdir(ctx, csdserver.CreateReq{
			VolumeID: parent.VolumeID, ParentID: msg.ParentID,
			Name: msg.Name, ModeBits: msg.Mode,
		})
		if err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgMkdirResp, toInodeMsg(inode))

	case MsgUnlink:
		var msg UnlinkMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		parent, err := srv.Metadata.Getattr(ctx, msg.ParentID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		if err := srv.Metadata.Unlink(ctx, csdserver.UnlinkReq{
			VolumeID: parent.VolumeID, InodeID: msg.InodeID,
			ParentID: msg.ParentID, Name: msg.Name,
		}); err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgUnlinkResp, struct{}{})

	case MsgRename:
		var msg RenameMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		inode, err := srv.Metadata.Getattr(ctx, msg.InodeID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		if err := srv.Metadata.Rename(ctx, csdserver.RenameReq{
			VolumeID:    inode.VolumeID,
			InodeID:     msg.InodeID,
			OldParentID: msg.OldParentID, OldName: msg.OldName,
			NewParentID: msg.NewParentID, NewName: msg.NewName,
		}); err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgRenameResp, struct{}{})

	case MsgTruncate:
		var msg TruncateMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		inode, err := srv.Metadata.Getattr(ctx, msg.InodeID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		if err := srv.Metadata.Truncate(ctx, csdserver.TruncateReq{
			VolumeID: inode.VolumeID, InodeID: msg.InodeID, NewSize: msg.NewSize,
		}); err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgTruncateResp, struct{}{})

	case MsgRead:
		var msg ReadMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		inode, err := srv.Metadata.Getattr(ctx, msg.InodeID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		data, err := srv.Extents.Read(ctx, csdserver.ReadReq{
			VolumeID: inode.VolumeID, InodeID: msg.InodeID,
			Offset: msg.Offset, Length: msg.Length,
		})
		if err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgReadResp, ReadRespMsg{Data: data, EOF: int64(msg.Length)+msg.Offset >= inode.SizeBytes})

	case MsgWrite:
		var msg WriteMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		inode, err := srv.Metadata.Getattr(ctx, msg.InodeID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		v, err := srv.Volumes.Get(ctx, inode.VolumeID, "")
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		if msg.Epoch != v.Epoch {
			return WriteError(w, ErrCodeStaleEpoch, "stale epoch", v.Epoch)
		}
		if err := srv.Extents.Write(ctx, csdserver.WriteReq{
			VolumeID:  inode.VolumeID,
			InodeID:   msg.InodeID,
			Offset:    msg.Offset,
			Data:      msg.Data,
			ClientID:  msg.ClientID,
			SessionID: msg.SessionID,
			Epoch:     msg.Epoch,
			OpSeq:     msg.OpSeq,
		}); err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgWriteResp, WriteRespMsg{Written: len(msg.Data)})

	case MsgLeaseAcquire:
		var msg LeaseAcquireMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		inode, err := srv.Metadata.Getattr(ctx, msg.InodeID)
		if err != nil {
			return WriteError(w, ErrCodeNotFound, err.Error(), 0)
		}
		lease, err := srv.Leases.Acquire(ctx, csdserver.LeaseRequest{
			VolumeID:   inode.VolumeID,
			InodeID:    msg.InodeID,
			ClientID:   msg.SessionID,
			SessionID:  msg.SessionID,
			LeaseType:  msg.LeaseType,
			RangeStart: msg.RangeStart,
			RangeEnd:   msg.RangeEnd,
			Epoch:      msg.Epoch,
		})
		if err == csd.ErrLeaseConflict {
			return WriteError(w, ErrCodeLeaseConflict, err.Error(), 0)
		}
		if err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgLeaseAcqResp, LeaseAcqRespMsg{
			LeaseID:   lease.ID,
			ExpiresAt: lease.ExpiresAt.Format(time.RFC3339),
			Epoch:     lease.Epoch,
		})

	case MsgLeaseRenew:
		var msg LeaseRenewMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		if err := srv.Leases.Renew(ctx, msg.LeaseID, msg.SessionID); err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgLeaseRenResp, struct{}{})

	case MsgLeaseRelease:
		var msg LeaseReleaseMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		if err := srv.Leases.Release(ctx, msg.LeaseID, msg.SessionID); err != nil {
			return WriteError(w, ErrCodeInternal, err.Error(), 0)
		}
		return WriteFrame(w, MsgLeaseRelResp, struct{}{})

	case MsgPing:
		var msg PingMsg
		if err := Decode(payload, &msg); err != nil {
			return err
		}
		return WriteFrame(w, MsgPong, PongMsg{Seq: msg.Seq})

	case MsgFsync:
		return WriteFrame(w, MsgFsyncResp, struct{}{})

	default:
		return WriteError(w, ErrCodeInternal, fmt.Sprintf("unknown message type 0x%02x", msgType), 0)
	}
}

// ---- helpers ----------------------------------------------------------------

func toInodeMsg(n csd.Inode) InodeMsg {
	return InodeMsg{
		ID:         n.ID,
		ParentID:   n.ParentID,
		Name:       n.Name,
		Type:       n.Type,
		Size:       n.SizeBytes,
		Mode:       n.ModeBits,
		UID:        n.UID,
		GID:        n.GID,
		ModifiedAt: n.ModifiedAt,
		CreatedAt:  n.CreatedAt,
	}
}

// CSDServerTLS returns a *tls.Config for a CSD QUIC server using a cert signed
// by the Capper internal CA. nodeID is used as the TLS CN and SAN.
func CSDServerTLS(certMgr *cert.Manager, nodeID string) (*tls.Config, error) {
	certPEM, keyPEM, err := certMgr.IssueNodeCert(nodeID, []string{nodeID}, 365*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("CSD: issue node cert: %w", err)
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	caPool, err := certMgr.CAPool()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		NextProtos:   []string{"csdp/1"},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// CSDClientTLS returns a *tls.Config for a CSD QUIC client that verifies
// the server certificate against the Capper CA.
func CSDClientTLS(certMgr *cert.Manager, nodeID string) (*tls.Config, error) {
	certPEM, keyPEM, err := certMgr.IssueNodeCert(nodeID+"-client", nil, 365*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("CSD: issue client cert: %w", err)
	}
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	caPool, err := certMgr.CAPool()
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      caPool,
		NextProtos:   []string{"csdp/1"},
		MinVersion:   tls.VersionTLS13,
	}, nil
}

// ClientTLSConfig returns a client-side TLS config using the provided CA pool.
// Pass nil caPool only in tests — production callers must supply a real pool.
func ClientTLSConfig(serverName string) *tls.Config {
	return &tls.Config{
		NextProtos: []string{"csdp/1"},
		ServerName: serverName,
		MinVersion: tls.VersionTLS13,
	}
}

// QUICTransport is the client-side QUIC connection to a CSD server.
type QUICTransport struct {
	conn    *quic.Conn
	session HelloOKMsg
}

// Dial connects to a CSD server and completes the Hello handshake.
func Dial(ctx context.Context, addr string, tlsCfg *tls.Config, hello HelloMsg) (*QUICTransport, HelloOKMsg, error) {
	conn, err := quic.DialAddr(ctx, addr, tlsCfg, &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	})
	if err != nil {
		return nil, HelloOKMsg{}, fmt.Errorf("csdp: dial %s: %w", addr, err)
	}

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, HelloOKMsg{}, fmt.Errorf("csdp: open hello stream: %w", err)
	}
	if err := WriteFrame(stream, MsgHello, hello); err != nil {
		_ = stream.Close()
		_ = conn.CloseWithError(0, "")
		return nil, HelloOKMsg{}, err
	}
	msgType, payload, err := ReadFrame(stream)
	_ = stream.Close()
	if err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, HelloOKMsg{}, err
	}
	if msgType == MsgError {
		var e ErrorMsg
		_ = Decode(payload, &e)
		_ = conn.CloseWithError(0, "")
		return nil, HelloOKMsg{}, fmt.Errorf("csdp: hello rejected: %s: %s", e.Code, e.Message)
	}
	if msgType != MsgHelloOK {
		_ = conn.CloseWithError(0, "")
		return nil, HelloOKMsg{}, fmt.Errorf("csdp: unexpected response type 0x%02x", msgType)
	}
	var ok HelloOKMsg
	if err := Decode(payload, &ok); err != nil {
		_ = conn.CloseWithError(0, "")
		return nil, HelloOKMsg{}, err
	}
	t := &QUICTransport{conn: conn, session: ok}
	return t, ok, nil
}

// Call opens a new stream, sends a request, and reads a response.
func (t *QUICTransport) Call(ctx context.Context, msgType byte, req, resp any) error {
	stream, err := t.conn.OpenStreamSync(ctx)
	if err != nil {
		return fmt.Errorf("csdp: open stream: %w", err)
	}
	defer stream.Close()
	if err := WriteFrame(stream, msgType, req); err != nil {
		return err
	}
	rType, payload, err := ReadFrame(stream)
	if err != nil {
		return err
	}
	if rType == MsgError {
		var e ErrorMsg
		_ = Decode(payload, &e)
		if e.Code == ErrCodeStaleEpoch {
			return fmt.Errorf("%w: server epoch=%d", csd.ErrStaleEpoch, e.Epoch)
		}
		return fmt.Errorf("csdp error %s: %s", e.Code, e.Message)
	}
	if resp != nil {
		return Decode(payload, resp)
	}
	return nil
}

func (t *QUICTransport) Close() error {
	return t.conn.CloseWithError(0, "bye")
}

func (t *QUICTransport) Session() HelloOKMsg { return t.session }

