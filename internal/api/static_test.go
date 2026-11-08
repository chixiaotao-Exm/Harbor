package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStaticCaching(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "assets"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>shell</html>"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "assets", "app-abc.js"), []byte("console.log(1)"), 0o644)

	s := newTestServer(t)
	s.WebDir = dir
	h := s.static()

	rec := &recorder{}
	h.ServeHTTP(rec, mustReq("GET", "/assets/app-abc.js"))
	if got := rec.hdr.Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("asset should be cached long: %q", got)
	}

	rec = &recorder{}
	h.ServeHTTP(rec, mustReq("GET", "/some/route"))
	if got := rec.hdr.Get("Cache-Control"); !strings.Contains(got, "no-cache") {
		t.Fatalf("shell should revalidate: %q", got)
	}
}
