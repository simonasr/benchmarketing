package controller

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUIServing(t *testing.T) {
	_, err := getUIFS()
	if err != nil {
		t.Fatalf("expected embedded UI fs, got error: %v", err)
	}

	handler := http.FileServer(http.FS(mustGetUIFS(t)))

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	rr := httptest.NewRecorder()

	// simulate the /ui/ strip prefix route
	http.StripPrefix("/ui/", handler).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("expected a content-type, got empty")
	}
}

func mustGetUIFS(t *testing.T) fs.FS {
	t.Helper()
	f, err := getUIFS()
	if err != nil {
		t.Fatal(err)
	}
	return f
}
