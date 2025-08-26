package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// RegistrationClient handles worker registration with the controller.
type RegistrationClient struct {
	controllerURL string
	workerID      string
	workerAddress string
	workerPort    int
	httpClient    *http.Client
}

// RegistrationRequest represents a worker registration request.
type RegistrationRequest struct {
	WorkerID string `json:"workerId"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
}

// NewRegistrationClient creates a new registration client.
func NewRegistrationClient(controllerURL, workerID, workerAddress string, workerPort int) *RegistrationClient {
	return &RegistrationClient{
		controllerURL: controllerURL,
		workerID:      workerID,
		workerAddress: workerAddress,
		workerPort:    workerPort,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Register registers the worker with the controller.
func (rc *RegistrationClient) Register() error {
	req := RegistrationRequest{
		WorkerID: rc.workerID,
		Address:  rc.workerAddress,
		Port:     rc.workerPort,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal registration request: %w", err)
	}

	url := fmt.Sprintf("%s/workers/register", rc.controllerURL)
	resp, err := rc.httpClient.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to send registration request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("registration failed with status %d", resp.StatusCode)
	}

	slog.Info("Worker registered successfully", "worker_id", rc.workerID, "controller", rc.controllerURL)
	return nil
}

// Unregister unregisters the worker from the controller.
func (rc *RegistrationClient) Unregister() error {
	url := fmt.Sprintf("%s/workers/%s", rc.controllerURL, rc.workerID)
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create unregistration request: %w", err)
	}

	resp, err := rc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send unregistration request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("unregistration failed with status %d", resp.StatusCode)
	}

	slog.Info("Worker unregistered", "worker_id", rc.workerID, "controller", rc.controllerURL)
	return nil
}
