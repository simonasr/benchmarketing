package httpx

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWriteJSON_OK(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusOK, map[string]string{"k": "v"})
	res := rec.Result()
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("unexpected content-type: %s", ct)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", res.StatusCode)
	}
	// body should be valid JSON
	if _, err := io.ReadAll(res.Body); err != nil {
		t.Fatalf("reading body: %v", err)
	}
}

func TestCheckMethod(t *testing.T) {
	h := func(w http.ResponseWriter, r *http.Request) {
		if !CheckMethod(w, r, http.MethodPost) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestLogAndRespond(t *testing.T) {
	rec := httptest.NewRecorder()
	LogAndRespond(rec, "msg", errors.New("boom"), "bad", http.StatusBadRequest)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestNewServer_Defaults(t *testing.T) {
	mux := http.NewServeMux()
	s := NewServer(":0", mux, 0, 0, 0)
	if s.ReadTimeout != DefaultReadTimeout {
		t.Fatalf("unexpected read timeout: %v", s.ReadTimeout)
	}
	if s.WriteTimeout != DefaultWriteTimeout {
		t.Fatalf("unexpected write timeout: %v", s.WriteTimeout)
	}
	if s.IdleTimeout != DefaultIdleTimeout {
		t.Fatalf("unexpected idle timeout: %v", s.IdleTimeout)
	}
}

func TestNewServer_Custom(t *testing.T) {
	mux := http.NewServeMux()
	read := 1 * time.Second
	write := 2 * time.Second
	idle := 3 * time.Second
	s := NewServer(":0", mux, read, write, idle)
	if s.ReadTimeout != read || s.WriteTimeout != write || s.IdleTimeout != idle {
		t.Fatalf("custom timeouts not applied")
	}
}
