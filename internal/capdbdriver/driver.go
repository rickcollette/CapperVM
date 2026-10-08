//go:build capdb

package capdbdriver

/*
// CapDB lives in an external tree (see CAPDB_DIR in the Makefile). The client
// include path and the libcapdb_client.a static lib are supplied at build time
// via CGO_CFLAGS / CGO_LDFLAGS by `make build-capdb` / `make test-capdb`; only
// the portable flags live here.
#cgo CFLAGS: -DCAPDB_ENABLE_NETWORK=1
#cgo LDFLAGS: -lssl -lcrypto -lpthread
#include <stdlib.h>
#include "capdb_client.h"

// Thin wrapper so Go never has to pass a C function pointer for the
// (unused) row callback.
static int capdb_exec_noload(capdb_conn *p, const char *zSql){
  return capdb_net_exec(p, zSql, 0, 0);
}
*/
import "C"

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Wire value-type tags returned by capdb_net_column_type (capdb_proto.h).
const (
	valNull  = 0
	valInt   = 1
	valFloat = 2
	valText  = 3
	valBlob  = 4
)

// Network status codes (capdb_client.h).
const (
	netOK       = 0
	netBusy     = 5
	netAuthFail = 2
)

func init() {
	sql.Register("capdb", Driver{})
}

// Driver is the database/sql driver for CapDB.
type Driver struct{}

func (Driver) Open(dsn string) (driver.Conn, error) {
	c := &conn{}
	curi := C.CString(dsn)
	defer C.free(unsafe.Pointer(curi)) // nosemgrep: go.lang.security.audit.unsafe.use-of-unsafe-block
	rc := C.capdb_net_connect(curi, &c.h)
	if rc != netOK || c.h == nil {
		msg := errmsg(c.h)
		if c.h != nil {
			C.capdb_net_close(c.h)
		}
		return nil, fmt.Errorf("capdb: connect failed (rc=%d): %s", int(rc), msg)
	}
	return c, nil
}

// OpenConnector lets database/sql reuse one parsed DSN across the pool.
func (d Driver) OpenConnector(dsn string) (driver.Connector, error) {
	return &connector{dsn: dsn, d: d}, nil
}

type connector struct {
	dsn string
	d   Driver
}

func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.d.Open(c.dsn)
}

func (c *connector) Driver() driver.Driver { return c.d }

// conn wraps a single capdb_conn. database/sql guarantees a conn is used by at
// most one goroutine at a time; the mutex only guards against the (illegal but
// cheap-to-defend) overlap and makes the race detector happy.
type conn struct {
	mu   sync.Mutex
	h    *C.capdb_conn
	dead atomic.Bool // set when a context cancellation aborted an in-flight call
}

// IsValid reports whether the pool may reuse this connection (driver.Validator).
// A connection whose socket was shut down by a context cancellation is discarded.
func (c *conn) IsValid() bool { return !c.dead.Load() && c.h != nil }

// watch aborts the current blocking call if ctx is cancelled, by shutting down
// the socket so the pending exec/step returns. It returns a stop func to call
// once the operation completes. For a non-cancellable context it is a no-op.
func (c *conn) watch(ctx context.Context) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c.dead.Store(true)
			C.capdb_net_cancel(c.h)
		case <-stop:
		}
	}()
	return func() { close(stop) }
}

// namedValues flattens positional driver.NamedValue args to driver.Value.
func namedValues(nv []driver.NamedValue) []driver.Value {
	vs := make([]driver.Value, len(nv))
	for i := range nv {
		vs[i] = nv[i].Value
	}
	return vs
}

func errmsg(h *C.capdb_conn) string {
	if h == nil {
		return "no connection"
	}
	return C.GoString(C.capdb_net_errmsg(h))
}

func (c *conn) err(rc C.int) error {
	// Transport failure (peer reset, server restart, timeout): the socket is
	// dead. Mark the conn so IsValid() evicts it from the pool and the *next*
	// call gets a fresh connection — i.e. the pool self-heals. We deliberately do
	// NOT return driver.ErrBadConn here: this statement may already have executed
	// on the server (write committed, reply lost), and ErrBadConn would let
	// database/sql retry it on another conn, risking a double-write.
	if c.h == nil || C.capdb_net_alive(c.h) == 0 {
		c.dead.Store(true)
		return fmt.Errorf("capdb: connection lost (transport error)")
	}
	// BUSY means the statement did not run (lock contention) — safe to retry, so
	// signal ErrBadConn for transparent database/sql retry.
	if int(rc) == netBusy {
		return fmt.Errorf("capdb: %s: %w", errmsg(c.h), driver.ErrBadConn)
	}
	msg := errmsg(c.h)
	if msg == "" {
		switch int(rc) {
		case 19:
			msg = "UNIQUE constraint failed"
		case 787:
			msg = "FOREIGN KEY constraint failed"
		}
	}
	return fmt.Errorf("capdb: %s (rc=%d)", msg, int(rc))
}

func (c *conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.h != nil {
		C.capdb_net_close(c.h)
		c.h = nil
	}
	return nil
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return &stmt{c: c, query: query, n: countPlaceholders(query)}, nil
}

// PrepareContext is client-side only (no round-trip), so it just honors an
// already-cancelled context.
func (c *conn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.Prepare(query)
}

// Begin starts a real interactive transaction. The server pins a pooled
// connection to this session on BEGIN and suppresses per-statement auto-commit
// until COMMIT/ROLLBACK, so the transaction is atomic (Rollback truly undoes).
func (c *conn) Begin() (driver.Tx, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.exec("BEGIN"); err != nil {
		return nil, err
	}
	return &capdbTx{c: c}, nil
}

// exec runs a statement that returns no rows (DDL/DML, possibly multi-statement).
// On success the server's changes() and last_insert_rowid() are captured on the
// connection (read via capdb_net_changes / capdb_net_last_insert_rowid).
func (c *conn) exec(sqlText string) error {
	csql := C.CString(sqlText)
	defer C.free(unsafe.Pointer(csql)) // nosemgrep: go.lang.security.audit.unsafe.use-of-unsafe-block
	rc := C.capdb_exec_noload(c.h, csql)
	if int(rc) != netOK {
		return c.err(rc)
	}
	return nil
}

// ---- statements ----

type stmt struct {
	c     *conn
	query string
	n     int
}

func (s *stmt) Close() error  { return nil }
func (s *stmt) NumInput() int { return s.n }

func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.doExec(args)
}

// ExecContext aborts the in-flight statement if ctx is cancelled.
func (s *stmt) ExecContext(ctx context.Context, nargs []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	stop := s.c.watch(ctx)
	defer stop()
	res, err := s.doExec(namedValues(nargs))
	if cerr := ctx.Err(); cerr != nil {
		return nil, cerr
	}
	return res, err
}

func (s *stmt) doExec(args []driver.Value) (driver.Result, error) {
	sqlText, err := substitute(s.query, args)
	if err != nil {
		return nil, err
	}
	if err := s.c.exec(sqlText); err != nil {
		return nil, err
	}
	return result{
		rows:   int64(C.capdb_net_changes(s.c.h)),
		lastID: int64(C.capdb_net_last_insert_rowid(s.c.h)),
	}, nil
}

func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	return s.doQuery(context.Background(), args)
}

// QueryContext aborts the query (and its row iteration) if ctx is cancelled.
func (s *stmt) QueryContext(ctx context.Context, nargs []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.c.mu.Lock()
	defer s.c.mu.Unlock()
	stop := s.c.watch(ctx)
	r, err := s.doQuery(ctx, namedValues(nargs))
	if cerr := ctx.Err(); cerr != nil {
		stop()
		return nil, cerr
	}
	if err != nil {
		stop()
		return nil, err
	}
	// Hand the watcher's stop to the rows so cancellation stays armed during
	// iteration; it fires on the first Close.
	r.stop = stop
	return r, nil
}

func (s *stmt) doQuery(ctx context.Context, args []driver.Value) (*rows, error) {
	sqlText, err := substitute(s.query, args)
	if err != nil {
		return nil, err
	}
	csql := C.CString(sqlText)
	defer C.free(unsafe.Pointer(csql)) // nosemgrep: go.lang.security.audit.unsafe.use-of-unsafe-block
	var st *C.capdb_net_stmt
	if rc := C.capdb_net_prepare(s.c.h, csql, &st); int(rc) != netOK || st == nil {
		return nil, s.c.err(rc)
	}
	ncol := int(C.capdb_net_column_count(st))
	cols := make([]string, ncol)
	for i := range cols {
		// CapDB's net protocol does not transmit column names; Capper scans
		// positionally, so synthetic names are sufficient.
		cols[i] = fmt.Sprintf("col%d", i)
	}
	return &rows{c: s.c, st: st, cols: cols, ctx: ctx}, nil
}

// ---- result ----

type result struct {
	rows   int64
	lastID int64
}

func (r result) LastInsertId() (int64, error) { return r.lastID, nil }
func (r result) RowsAffected() (int64, error) { return r.rows, nil }

// ---- rows ----

type rows struct {
	c    *conn
	st   *C.capdb_net_stmt
	cols []string
	ctx  context.Context
	stop func() // stops the context-cancellation watcher (QueryContext only)
}

func (r *rows) Columns() []string { return r.cols }

func (r *rows) Close() error {
	if r.stop != nil {
		r.stop()
		r.stop = nil
	}
	if r.st != nil {
		C.capdb_net_finalize(r.st)
		r.st = nil
	}
	return nil
}

func (r *rows) Next(dest []driver.Value) error {
	if r.ctx != nil {
		if err := r.ctx.Err(); err != nil {
			return err
		}
	}
	if r.st == nil {
		return fmt.Errorf("capdb: rows already closed")
	}
	if r.c.h == nil || C.capdb_net_alive(r.c.h) == 0 {
		r.c.dead.Store(true)
		return fmt.Errorf("capdb: connection lost (transport error)")
	}
	rc := C.capdb_net_step(r.st)
	switch int(rc) {
	case 100: // CAPDB_ROW
		// fall through
	case 101: // CAPDB_DONE
		return io.EOF
	default:
		return r.c.err(rc)
	}
	for i := range dest {
		dest[i] = r.column(i)
	}
	return nil
}

func (r *rows) column(i int) driver.Value {
	ci := C.int(i)
	switch int(C.capdb_net_column_type(r.st, ci)) {
	case valNull:
		return nil
	case valInt:
		return int64(C.capdb_net_column_int64(r.st, ci))
	case valFloat:
		return float64(C.capdb_net_column_double(r.st, ci))
	case valBlob:
		n := C.capdb_net_column_bytes(r.st, ci)
		p := C.capdb_net_column_blob(r.st, ci)
		if p == nil || n == 0 {
			return []byte{}
		}
		return C.GoBytes(p, n)
	default: // valText and anything unexpected
		n := C.capdb_net_column_bytes(r.st, ci)
		p := C.capdb_net_column_text(r.st, ci)
		if p == nil {
			return nil
		}
		return C.GoStringN((*C.char)(unsafe.Pointer(p)), n) // nosemgrep: go.lang.security.audit.unsafe.use-of-unsafe-block
	}
}

// ---- tx ----

// capdbTx is a real transaction: Commit/Rollback issue COMMIT/ROLLBACK on the
// session's pinned connection. database/sql routes all of the transaction's
// statements through this same conn, so they share the pinned server-side
// connection and commit or roll back atomically.
type capdbTx struct{ c *conn }

func (t *capdbTx) Commit() error {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	return t.c.exec("COMMIT")
}

func (t *capdbTx) Rollback() error {
	t.c.mu.Lock()
	defer t.c.mu.Unlock()
	return t.c.exec("ROLLBACK")
}
