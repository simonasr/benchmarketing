package worker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewRegistrationClient(t *testing.T) {
	client := NewRegistrationClient("http://localhost:8080", "worker-1", "localhost", 8081)

	if client.controllerURL != "http://localhost:8080" {
		t.Errorf("Expected controller URL 'http://localhost:8080', got %s", client.controllerURL)
	}
	if client.workerID != "worker-1" {
		t.Errorf("Expected worker ID 'worker-1', got %s", client.workerID)
	}
	if client.workerAddress != "localhost" {
		t.Errorf("Expected worker address 'localhost', got %s", client.workerAddress)
	}
	if client.workerPort != 8081 {
		t.Errorf("Expected worker port 8081, got %d", client.workerPort)
	}
	if client.httpClient == nil {
		t.Error("HTTP client should be initialized")
	}
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "successful registration",
			responseStatus: http.StatusCreated,
			responseBody:   `{"status": "registered", "workerId": "worker-1"}`,
			wantErr:        false,
		},
		{
			name:           "registration conflict",
			responseStatus: http.StatusConflict,
			responseBody:   `{"error": "worker already exists"}`,
			wantErr:        true,
			errContains:    "registration failed with status 409",
		},
		{
			name:           "server error",
			responseStatus: http.StatusInternalServerError,
			responseBody:   `{"error": "internal server error"}`,
			wantErr:        true,
			errContains:    "registration failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}
				if r.URL.Path != "/workers/register" {
					t.Errorf("Expected path '/workers/register', got %s", r.URL.Path)
				}
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("Expected Content-Type 'application/json', got %s", r.Header.Get("Content-Type"))
				}

				// Verify request body
				var req RegistrationRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("Failed to decode request body: %v", err)
				}
				if req.WorkerID != "worker-1" {
					t.Errorf("Expected worker ID 'worker-1', got %s", req.WorkerID)
				}
				if req.Address != "localhost" {
					t.Errorf("Expected address 'localhost', got %s", req.Address)
				}
				if req.Port != 8081 {
					t.Errorf("Expected port 8081, got %d", req.Port)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client and test registration
			client := NewRegistrationClient(server.URL, "worker-1", "localhost", 8081)
			err := client.Register()

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestUnregister(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "successful unregistration",
			responseStatus: http.StatusOK,
			responseBody:   `{"status": "unregistered", "workerId": "worker-1"}`,
			wantErr:        false,
		},
		{
			name:           "worker not found",
			responseStatus: http.StatusNotFound,
			responseBody:   `{"error": "worker not found"}`,
			wantErr:        false, // 404 is treated as success
		},
		{
			name:           "server error",
			responseStatus: http.StatusInternalServerError,
			responseBody:   `{"error": "internal server error"}`,
			wantErr:        true,
			errContains:    "unregistration failed with status 500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method and path
				if r.Method != http.MethodDelete {
					t.Errorf("Expected DELETE method, got %s", r.Method)
				}
				expectedPath := "/workers/worker-1"
				if r.URL.Path != expectedPath {
					t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client and test unregistration
			client := NewRegistrationClient(server.URL, "worker-1", "localhost", 8081)
			err := client.Unregister()

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error to contain %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRegisterNetworkError(t *testing.T) {
	// Test with invalid URL to simulate network error
	client := NewRegistrationClient("http://invalid-url:99999", "worker-1", "localhost", 8081)
	err := client.Register()

	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "failed to send registration request") {
		t.Errorf("Expected network error, got %v", err)
	}
}

func TestUnregisterNetworkError(t *testing.T) {
	// Test with invalid URL to simulate network error
	client := NewRegistrationClient("http://invalid-url:99999", "worker-1", "localhost", 8081)
	err := client.Unregister()

	if err == nil {
		t.Error("Expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "failed to send unregistration request") {
		t.Errorf("Expected network error, got %v", err)
	}
}

func TestRegistrationRequestStructure(t *testing.T) {
	// Test that RegistrationRequest can be properly marshaled/unmarshaled
	req := RegistrationRequest{
		WorkerID: "worker-1",
		Address:  "localhost",
		Port:     8081,
	}

	// Marshal to JSON
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal registration request: %v", err)
	}

	// Unmarshal back
	var unmarshaled RegistrationRequest
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal registration request: %v", err)
	}

	// Verify fields
	if unmarshaled.WorkerID != req.WorkerID {
		t.Errorf("Expected worker ID %s, got %s", req.WorkerID, unmarshaled.WorkerID)
	}
	if unmarshaled.Address != req.Address {
		t.Errorf("Expected address %s, got %s", req.Address, unmarshaled.Address)
	}
	if unmarshaled.Port != req.Port {
		t.Errorf("Expected port %d, got %d", req.Port, unmarshaled.Port)
	}
}
