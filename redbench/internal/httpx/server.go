package httpx

import (
	"net/http"
	"time"
)

// NewServer creates an http.Server with default timeouts if zero values are provided.
func NewServer(addr string, handler http.Handler, read, write, idle time.Duration) *http.Server {
	if read == 0 {
		read = DefaultReadTimeout
	}
	if write == 0 {
		write = DefaultWriteTimeout
	}
	if idle == 0 {
		idle = DefaultIdleTimeout
	}
	return &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  read,
		WriteTimeout: write,
		IdleTimeout:  idle,
	}
}
