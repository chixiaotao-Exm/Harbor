package api

import (
	"io"
	"strings"
)

// readCloser 让测试能塞请求体。
type readCloser struct {
	s string
	i int
}

func (r *readCloser) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}

func (r *readCloser) Close() error { return nil }

var _ = strings.TrimSpace
