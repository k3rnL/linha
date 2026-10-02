package webui

import (
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestBuiltAssetsAndScopedFallback(t *testing.T) {
	// CI runs the production build before this test. Plain Go-only development may omit it.
	if _, e := os.Stat("assets/index.html"); e != nil {
		t.Skip("npm --prefix web run build required for embedded asset tests")
	}
	h, e := Handler()
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/ui/", "/ui/contexts/context", "/ui/requests/request"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<html") {
			t.Fatalf("%s: %d", path, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal("entry cached indefinitely")
		}
	}
	files, e := os.ReadDir("assets/assets")
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/ui/assets/"+files[0].Name(), nil))
	if w.Code != 200 || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("hashed asset cache missing")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/ui/assets/missing.js", nil))
	if w.Code != 404 {
		t.Fatal("missing script returned entry HTML")
	}
}
