package iox

import (
	"fmt"
	"io"
)

// DefaultMaxArchiveEntry is the per-entry limit for unpacking tar/zip streams
// from capsules, SBOMs, and similar archives (512 MiB).
const DefaultMaxArchiveEntry = 512 << 20

// CopyLimited copies from src to dst, stopping after max+1 bytes. If more than
// max bytes would be copied it returns an error (decompression bomb / zip bomb
// protection).
func CopyLimited(dst io.Writer, src io.Reader, max int64) (int64, error) {
	if max <= 0 {
		max = DefaultMaxArchiveEntry
	}
	n, err := io.Copy(dst, io.LimitReader(src, max+1))
	if n > max {
		return n, fmt.Errorf("io: read exceeds limit of %d bytes", max)
	}
	return n, err
}
