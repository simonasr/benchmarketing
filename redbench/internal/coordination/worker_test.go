package coordination

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// MockBenchmarkService for testing
type MockBenchmarkService struct {
	mock.Mock
}

func (m *MockBenchmarkService) Start(req interface{}) error {
	args := m.Called(req)
	return args.Error(0)
}

func (m *MockBenchmarkService) GetStatus() interface{} {
	args := m.Called()
	return args.Get(0)
}

func TestNewWorkerClient(t *testing.T) {
	cfg := &config.Coordination{
		IsLeader:       false,
		LeaderURL:      "http://leader:8080",
		WorkerID:       "test-worker",
		WorkerHost:     "localhost",
		PollIntervalMs: 1000,
	}

	mockService := &MockBenchmarkService{}
	workerPort := 8090

	client := NewWorkerClient(cfg, mockService, workerPort)

	assert.NotNil(t, client)
	assert.Equal(t, "test-worker", client.workerID)
	assert.Equal(t, "http://localhost:8090", client.workerURL)
	assert.Equal(t, "http://leader:8080", client.leaderURL)
	assert.Equal(t, 1000*time.Millisecond, client.pollInterval)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, 10*time.Second, client.httpClient.Timeout)
	assert.NotNil(t, client.ctx)
	assert.NotNil(t, client.cancel)

	// Clean up
	client.Shutdown()
}

func TestNewWorkerClient_DefaultHost(t *testing.T) {
	cfg := &config.Coordination{
		WorkerID:   "test-worker",
		WorkerHost: "", // Empty host should default to localhost
	}

	mockService := &MockBenchmarkService{}
	client := NewWorkerClient(cfg, mockService, 8090)

	assert.Equal(t, "http://localhost:8090", client.workerURL)
	client.Shutdown()
}

func TestWorkerClient_RegisterWithLeader(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantErr        bool
		expectedError  string
	}{
		{
			name: "successful registration",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/workers/register", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Verify body
				var reg WorkerRegistration
				err := json.NewDecoder(r.Body).Decode(&reg)
				assert.NoError(t, err)
				assert.Equal(t, "test-worker", reg.WorkerID)
				assert.Equal(t, "http://localhost:8090", reg.URL)

				w.WriteHeader(http.StatusOK)
			},
			wantErr: false,
		},
		{
			name: "registration failure",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:       true,
			expectedError: "registration failed with status: 500",
		},
		{
			name: "server unavailable",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Server will be closed to simulate network error
			},
			wantErr:       true,
			expectedError: "failed to register",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server

			if tt.name != "server unavailable" {
				server = httptest.NewServer(http.HandlerFunc(tt.serverResponse))
				defer server.Close()
			} else {
				server = httptest.NewServer(http.HandlerFunc(tt.serverResponse))
				server.Close() // Close immediately to simulate unavailable server
			}

			cfg := &config.Coordination{
				WorkerID:   "test-worker",
				WorkerHost: "localhost",
				LeaderURL:  server.URL,
			}

			mockService := &MockBenchmarkService{}
			client := NewWorkerClient(cfg, mockService, 8090)
			defer client.Shutdown()

			err := client.registerWithLeader()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.expectedError != "" {
					assert.Contains(t, err.Error(), tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWorkerClient_GetAssignment(t *testing.T) {
	tests := []struct {
		name               string
		serverResponse     func(w http.ResponseWriter, r *http.Request)
		wantErr            bool
		expectedAssignment *Assignment
	}{
		{
			name: "assignment available",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				assert.Equal(t, "GET", r.Method)
				assert.Equal(t, "/workers/test-worker/assignment", r.URL.Path)

				assignment := &Assignment{
					ID: "assignment-1",
					RedisTarget: RedisTarget{
						Host: "redis1",
						Port: "6379",
					},
					Config: &TestConfig{
						MinClients: intPtr(1),
						MaxClients: intPtr(10),
					},
					CreatedAt: time.Now(),
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(assignment)
			},
			wantErr: false,
			expectedAssignment: &Assignment{
				ID: "assignment-1",
			},
		},
		{
			name: "no assignment available",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			wantErr:            false,
			expectedAssignment: nil,
		},
		{
			name: "server error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:            true,
			expectedAssignment: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Coordination{
				WorkerID:  "test-worker",
				LeaderURL: server.URL,
			}

			mockService := &MockBenchmarkService{}
			client := NewWorkerClient(cfg, mockService, 8090)
			defer client.Shutdown()

			assignment, err := client.getAssignment()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, assignment)
			} else {
				assert.NoError(t, err)
				if tt.expectedAssignment != nil {
					assert.NotNil(t, assignment)
					assert.Equal(t, tt.expectedAssignment.ID, assignment.ID)
				} else {
					assert.Nil(t, assignment)
				}
			}
		})
	}
}

func TestWorkerClient_UpdateStatus(t *testing.T) {
	tests := []struct {
		name           string
		assignmentID   string
		status         string
		err            error
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantRequestErr bool
	}{
		{
			name:         "successful status update",
			assignmentID: "assignment-1",
			status:       "running",
			err:          nil,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Verify request
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/workers/test-worker/status", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Verify body
				var statusUpdate WorkerStatus
				err := json.NewDecoder(r.Body).Decode(&statusUpdate)
				assert.NoError(t, err)
				assert.Equal(t, "test-worker", statusUpdate.WorkerID)
				assert.Equal(t, "assignment-1", statusUpdate.AssignmentID)
				assert.Equal(t, "running", statusUpdate.Status)

				w.WriteHeader(http.StatusOK)
			},
			wantRequestErr: false,
		},
		{
			name:         "status update with error",
			assignmentID: "assignment-1",
			status:       "failed",
			err:          fmt.Errorf("benchmark failed"),
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Verify error is included in request
				var statusUpdate WorkerStatus
				err := json.NewDecoder(r.Body).Decode(&statusUpdate)
				assert.NoError(t, err)
				assert.Equal(t, "benchmark failed", statusUpdate.Error)

				w.WriteHeader(http.StatusOK)
			},
			wantRequestErr: false,
		},
		{
			name:         "server error",
			assignmentID: "assignment-1",
			status:       "running",
			err:          nil,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantRequestErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Coordination{
				WorkerID:  "test-worker",
				LeaderURL: server.URL,
			}

			mockService := &MockBenchmarkService{}
			client := NewWorkerClient(cfg, mockService, 8090)
			defer client.Shutdown()

			client.updateStatus(tt.assignmentID, tt.status, tt.err)

			// Note: updateStatus doesn't return an error in the current implementation
			// In a real scenario, we might want to test the HTTP request was made correctly
			// by examining the mock server's received requests
		})
	}
}

func TestWorkerClient_StartBenchmark(t *testing.T) {
	tests := []struct {
		name               string
		assignment         *Assignment
		mockSetup          func(*MockBenchmarkService)
		serverResponse     func(w http.ResponseWriter, r *http.Request)
		expectStatusUpdate bool
	}{
		{
			name: "successful benchmark start",
			assignment: &Assignment{
				ID: "assignment-1",
				RedisTarget: RedisTarget{
					Host:  "redis1",
					Port:  "6379",
					Label: "Test Redis",
				},
				Config: &TestConfig{
					MinClients: intPtr(1),
					MaxClients: intPtr(10),
				},
			},
			mockSetup: func(m *MockBenchmarkService) {
				m.On("Start", mock.AnythingOfType("*coordination.StartRequest")).Return(nil)
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Accept status updates
				w.WriteHeader(http.StatusOK)
			},
			expectStatusUpdate: true,
		},
		{
			name: "benchmark start failure",
			assignment: &Assignment{
				ID: "assignment-1",
				RedisTarget: RedisTarget{
					Host: "redis1",
					Port: "6379",
				},
				Config: &TestConfig{},
			},
			mockSetup: func(m *MockBenchmarkService) {
				m.On("Start", mock.AnythingOfType("*coordination.StartRequest")).Return(fmt.Errorf("benchmark failed"))
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				// Accept status updates (can't assert in HTTP handler due to async nature)
				w.WriteHeader(http.StatusOK)
			},
			expectStatusUpdate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Coordination{
				WorkerID:  "test-worker",
				LeaderURL: server.URL,
			}

			mockService := &MockBenchmarkService{}
			tt.mockSetup(mockService)

			client := NewWorkerClient(cfg, mockService, 8090)
			defer client.Shutdown()

			client.startBenchmark(tt.assignment)

			// Give some time for async operations
			time.Sleep(10 * time.Millisecond)

			mockService.AssertExpectations(t)
		})
	}
}

func TestWorkerClient_ParseStatusFromInterface(t *testing.T) {
	cfg := &config.Coordination{WorkerID: "test-worker"}
	mockService := &MockBenchmarkService{}
	client := NewWorkerClient(cfg, mockService, 8090)
	defer client.Shutdown()

	tests := []struct {
		name            string
		statusInterface interface{}
		expectedStatus  string
		expectedError   string
	}{
		{
			name:            "nil status",
			statusInterface: nil,
			expectedStatus:  "status_unavailable",
			expectedError:   "nil status interface",
		},
		{
			name: "valid map status",
			statusInterface: map[string]interface{}{
				"status":        "running",
				"current_stage": 5,
				"max_stage":     10,
			},
			expectedStatus: "running",
		},
		{
			name: "valid struct status",
			statusInterface: map[string]interface{}{
				"status": "completed",
				"error":  "",
			},
			expectedStatus: "completed",
		},
		{
			name:            "invalid status type",
			statusInterface: "invalid",
			expectedStatus:  "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.parseStatusFromInterface(tt.statusInterface)

			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedStatus, result.Status)

			if tt.expectedError != "" {
				assert.Equal(t, tt.expectedError, result.Error)
			}
		})
	}
}

func TestWorkerClient_TestConfigToMap(t *testing.T) {
	cfg := &config.Coordination{WorkerID: "test-worker"}
	mockService := &MockBenchmarkService{}
	client := NewWorkerClient(cfg, mockService, 8090)
	defer client.Shutdown()

	tests := []struct {
		name     string
		config   *TestConfig
		expected map[string]interface{}
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: make(map[string]interface{}),
		},
		{
			name: "full config",
			config: &TestConfig{
				MinClients:     intPtr(1),
				MaxClients:     intPtr(10),
				StageIntervalS: intPtr(2),
				RequestDelayMs: intPtr(100),
				KeySize:        intPtr(8),
				ValueSize:      intPtr(64),
			},
			expected: map[string]interface{}{
				"min_clients":      1,
				"max_clients":      10,
				"stage_interval_s": 2,
				"request_delay_ms": 100,
				"key_size":         8,
				"value_size":       64,
			},
		},
		{
			name: "partial config",
			config: &TestConfig{
				MinClients: intPtr(5),
				MaxClients: intPtr(20),
			},
			expected: map[string]interface{}{
				"min_clients": 5,
				"max_clients": 20,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.testConfigToMap(tt.config)

			assert.Equal(t, len(tt.expected), len(result))
			for key, expectedValue := range tt.expected {
				assert.Equal(t, expectedValue, result[key])
			}
		})
	}
}

func TestWorkerClient_PerformCoordinationCycle(t *testing.T) {
	tests := []struct {
		name           string
		assignment     *Assignment
		serverResponse func(w http.ResponseWriter, r *http.Request)
		mockSetup      func(*MockBenchmarkService)
		expectStart    bool
	}{
		{
			name: "no assignment available",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/workers/test-worker/assignment" {
					w.WriteHeader(http.StatusNotFound)
				}
			},
			mockSetup:   func(m *MockBenchmarkService) {},
			expectStart: false,
		},
		{
			name: "assignment without start signal",
			assignment: &Assignment{
				ID:          "assignment-1",
				RedisTarget: RedisTarget{Host: "redis1", Port: "6379"},
				Config:      &TestConfig{},
				StartSignal: nil, // No start signal yet
			},
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/workers/test-worker/assignment" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(&Assignment{
						ID:          "assignment-1",
						RedisTarget: RedisTarget{Host: "redis1", Port: "6379"},
						Config:      &TestConfig{},
						StartSignal: nil,
					})
				}
			},
			mockSetup:   func(m *MockBenchmarkService) {},
			expectStart: false,
		},
		{
			name: "assignment with future start signal",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/workers/test-worker/assignment" {
					futureTime := time.Now().Add(1 * time.Hour)
					assignment := &Assignment{
						ID:          "assignment-1",
						RedisTarget: RedisTarget{Host: "redis1", Port: "6379"},
						Config:      &TestConfig{},
						StartSignal: &futureTime,
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(assignment)
				}
			},
			mockSetup:   func(m *MockBenchmarkService) {},
			expectStart: false,
		},
		{
			name: "assignment ready to start",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/workers/test-worker/assignment" {
					pastTime := time.Now().Add(-1 * time.Minute)
					assignment := &Assignment{
						ID:          "assignment-1",
						RedisTarget: RedisTarget{Host: "redis1", Port: "6379"},
						Config:      &TestConfig{},
						StartSignal: &pastTime,
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(assignment)
				} else {
					// Accept status updates
					w.WriteHeader(http.StatusOK)
				}
			},
			mockSetup: func(m *MockBenchmarkService) {
				m.On("Start", mock.AnythingOfType("*coordination.StartRequest")).Return(nil)
			},
			expectStart: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			cfg := &config.Coordination{
				WorkerID:  "test-worker",
				LeaderURL: server.URL,
			}

			mockService := &MockBenchmarkService{}
			tt.mockSetup(mockService)

			client := NewWorkerClient(cfg, mockService, 8090)
			defer client.Shutdown()

			client.performCoordinationCycle()

			// Give some time for async operations
			time.Sleep(10 * time.Millisecond)

			if tt.expectStart {
				mockService.AssertExpectations(t)
			} else {
				// Should not have called Start
				mockService.AssertNotCalled(t, "Start")
			}
		})
	}
}

func TestWorkerClient_Shutdown(t *testing.T) {
	cfg := &config.Coordination{WorkerID: "test-worker"}
	mockService := &MockBenchmarkService{}
	client := NewWorkerClient(cfg, mockService, 8090)

	// Verify context is not cancelled initially
	select {
	case <-client.ctx.Done():
		t.Fatal("Context should not be cancelled initially")
	default:
		// Expected
	}

	// Shutdown
	client.Shutdown()

	// Verify context is cancelled
	select {
	case <-client.ctx.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context should be cancelled after shutdown")
	}
}

func TestWorkerClient_Integration(t *testing.T) {
	// This test simulates a full coordination cycle

	// Create a mock leader server
	registrationReceived := make(chan bool, 1)
	assignmentRequested := make(chan bool, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/workers/register":
			// Handle registration
			var reg WorkerRegistration
			json.NewDecoder(r.Body).Decode(&reg)
			assert.Equal(t, "test-worker", reg.WorkerID)
			w.WriteHeader(http.StatusOK)
			registrationReceived <- true

		case r.Method == "GET" && r.URL.Path == "/workers/test-worker/assignment":
			// Handle assignment request
			assignmentRequested <- true
			w.WriteHeader(http.StatusNotFound) // No assignment

		case r.Method == "POST" && r.URL.Path == "/workers/test-worker/status":
			// Handle status update
			w.WriteHeader(http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := &config.Coordination{
		WorkerID:       "test-worker",
		LeaderURL:      server.URL,
		PollIntervalMs: 50, // Fast polling for test
	}

	mockService := &MockBenchmarkService{}
	client := NewWorkerClient(cfg, mockService, 8090)
	defer client.Shutdown()

	// Start the worker
	err := client.Start()
	require.NoError(t, err)

	// Verify registration happened
	select {
	case <-registrationReceived:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("Registration should have been received")
	}

	// Verify assignment polling starts
	select {
	case <-assignmentRequested:
		// Expected
	case <-time.After(1 * time.Second):
		t.Fatal("Assignment should have been requested")
	}
}
