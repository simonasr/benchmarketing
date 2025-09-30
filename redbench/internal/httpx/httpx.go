package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

const (
	DefaultReadTimeout  = 15 * time.Second
	DefaultWriteTimeout = 15 * time.Second
	DefaultIdleTimeout  = 60 * time.Second
)

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if statusCode != http.StatusOK {
		w.WriteHeader(statusCode)
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// LogAndRespond logs an error and sends an HTTP error response with provided status code.
func LogAndRespond(w http.ResponseWriter, logMsg string, err error, httpMsg string, statusCode int) {
	slog.Error(logMsg, "error", err)
	http.Error(w, httpMsg, statusCode)
}

// CheckMethod validates the HTTP method and returns false if invalid.
func CheckMethod(w http.ResponseWriter, r *http.Request, expectedMethod string) bool {
	if r.Method != expectedMethod {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}
