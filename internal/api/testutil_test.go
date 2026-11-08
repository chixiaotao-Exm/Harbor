package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
)

// recorder 是极简 ResponseWriter，够测试用。
type recorder struct {
	code int
	body string
	hdr  http.Header
}

func (r *recorder) Header() http.Header {
	if r.hdr == nil {
		r.hdr = http.Header{}
	}
	return r.hdr
}

func (r *recorder) Write(b []byte) (int, error) {
	r.body += string(b)
	return len(b), nil
}

func (r *recorder) WriteHeader(code int) { r.code = code }

func mustReq(method, path string) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewReader(nil))
}
