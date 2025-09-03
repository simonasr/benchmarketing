package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestControllerExitWorker_ProxiesAndUnregisters(t *testing.T) {
	// Fake worker server to capture /exit
	exitCalled := make(chan struct{}, 1)
	workerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exit" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		select {
		case exitCalled <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer workerSrv.Close()

	// Parse host:port from test server
	u, err := url.Parse(workerSrv.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	host := u.Hostname()
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	// Build controller and mux
	cfg := &config.Config{Controller: config.LoadControllerConfig()}
	c := NewController(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/workers/", c.WorkerHandler)

	// Register a worker pointing to the fake server
	workerID := "w-1"
	if err := c.registry.RegisterWorker(RegistrationRequest{WorkerID: workerID, Address: host, Port: port}); err != nil {
		t.Fatalf("register worker: %v", err)
	}

	// Call controller endpoint to exit worker
	req := httptest.NewRequest(http.MethodPost, "/workers/"+workerID+"/exit", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Ensure proxy hit worker /exit
	select {
	case <-exitCalled:
		// ok
	default:
		t.Fatal("expected worker /exit to be called")
	}

	// Ensure worker is unregistered
	if _, ok := c.registry.GetWorker(workerID); ok {
		t.Fatal("expected worker to be unregistered after exit")
	}
}
