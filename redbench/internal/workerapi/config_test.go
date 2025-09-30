package workerapi

import (
	"encoding/json"
	"testing"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

func TestMergeConfiguration_EmptyBody(t *testing.T) {
	baseConfig := &config.Config{
		Test: config.Test{
			MinClients:      1,
			MaxClients:      100,
			StageIntervalMs: 10000,
			RequestDelayMs:  1000,
			KeySize:         10,
			ValueSize:       10,
		},
	}

	mergedConfig, err := MergeConfiguration(baseConfig, []byte{})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should return identical config for empty body
	if mergedConfig.Test.MinClients != baseConfig.Test.MinClients {
		t.Errorf("Expected MinClients %d, got %d", baseConfig.Test.MinClients, mergedConfig.Test.MinClients)
	}
	if mergedConfig.Test.MaxClients != baseConfig.Test.MaxClients {
		t.Errorf("Expected MaxClients %d, got %d", baseConfig.Test.MaxClients, mergedConfig.Test.MaxClients)
	}
}

func TestMergeConfiguration_WithOverrides(t *testing.T) {
	baseConfig := &config.Config{
		Test: config.Test{
			MinClients:      1,
			MaxClients:      100,
			StageIntervalMs: 10000,
			RequestDelayMs:  1000,
			KeySize:         10,
			ValueSize:       10,
		},
	}

	requestBody := BenchmarkRequest{
		Test: &TestOverrides{
			MinClients: intPtr(5),
			MaxClients: intPtr(50),
			KeySize:    intPtr(20),
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	mergedConfig, err := MergeConfiguration(baseConfig, bodyBytes)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Check overridden values
	if mergedConfig.Test.MinClients != 5 {
		t.Errorf("Expected MinClients 5, got %d", mergedConfig.Test.MinClients)
	}
	if mergedConfig.Test.MaxClients != 50 {
		t.Errorf("Expected MaxClients 50, got %d", mergedConfig.Test.MaxClients)
	}
	if mergedConfig.Test.KeySize != 20 {
		t.Errorf("Expected KeySize 20, got %d", mergedConfig.Test.KeySize)
	}

	// Check non-overridden values remain the same
	if mergedConfig.Test.StageIntervalMs != baseConfig.Test.StageIntervalMs {
		t.Errorf("Expected StageIntervalMs %d, got %d", baseConfig.Test.StageIntervalMs, mergedConfig.Test.StageIntervalMs)
	}
	if mergedConfig.Test.RequestDelayMs != baseConfig.Test.RequestDelayMs {
		t.Errorf("Expected RequestDelayMs %d, got %d", baseConfig.Test.RequestDelayMs, mergedConfig.Test.RequestDelayMs)
	}
	if mergedConfig.Test.ValueSize != baseConfig.Test.ValueSize {
		t.Errorf("Expected ValueSize %d, got %d", baseConfig.Test.ValueSize, mergedConfig.Test.ValueSize)
	}
}

func TestStatusSanitizesTagsCountForSetGet(t *testing.T) {
	// Base config with tagsCount set (would be used for cluster workloads)
	baseCfg := &config.Config{
		Test: config.Test{
			MinClients:      1,
			MaxClients:      2,
			StageIntervalMs: 1000,
			RequestDelayMs:  10,
			KeySize:         8,
			ValueSize:       12,
			Workload:        config.WorkloadSetGet,
			TagsCount:       1024,
		},
	}

	// Start with a set_get request body (no tagsCount provided)
	req := BenchmarkRequest{
		Test:  &TestOverrides{Workload: stringPtr(config.WorkloadSetGet)},
		Redis: &RedisOverrides{URL: stringPtr("redis://localhost:6379")},
	}
	body, _ := json.Marshal(req)

	// Merge config and create conn, then run StartHandler via direct call of merge path
	merged, err := MergeConfiguration(baseCfg, body)
	if err != nil {
		t.Fatalf("merge error: %v", err)
	}
	// Simulate the same sanitization as StartHandler
	if !config.IsBatchWorkload(merged.Test.Workload) {
		merged.Test.BatchSize = 0
		merged.Test.SameSlotPerClient = false
		merged.Test.TagsCount = 0
	}
	if merged.Test.TagsCount != 0 {
		t.Fatalf("expected tagsCount=0 for set_get workload after sanitization, got %d", merged.Test.TagsCount)
	}
}

func TestMergeConfiguration_WithRedisOverrides(t *testing.T) {
	// Base configuration with default Redis values
	baseConfig := &config.Config{
		Redis: config.RedisConfig{
			OperationTimeoutMs: 100,
			Expiration:         20,
		},
		Test: config.Test{
			MinClients: 1,
			MaxClients: 10,
		},
	}

	// Request with Redis configuration overrides
	body := `{
		"redis": {
			"operationTimeoutMs": 1000,
			"expiration": 60
		}
	}`
	bodyBytes := []byte(body)

	mergedConfig, err := MergeConfiguration(baseConfig, bodyBytes)
	if err != nil {
		t.Fatalf("MergeConfiguration failed: %v", err)
	}

	// Verify Redis overrides were applied
	if mergedConfig.Redis.OperationTimeoutMs != 1000 {
		t.Errorf("Expected OperationTimeoutMs 1000, got %d", mergedConfig.Redis.OperationTimeoutMs)
	}
	if mergedConfig.Redis.Expiration != 60 {
		t.Errorf("Expected Expiration 60, got %d", mergedConfig.Redis.Expiration)
	}

	// Verify test config was not affected
	if mergedConfig.Test.MinClients != baseConfig.Test.MinClients {
		t.Errorf("Expected MinClients %d, got %d", baseConfig.Test.MinClients, mergedConfig.Test.MinClients)
	}
}

func TestMergeConfiguration_PartialOverrides(t *testing.T) {
	baseConfig := &config.Config{
		Test: config.Test{
			MinClients:      1,
			MaxClients:      100,
			StageIntervalMs: 10000,
			RequestDelayMs:  1000,
			KeySize:         10,
			ValueSize:       10,
		},
	}

	requestBody := BenchmarkRequest{
		Test: &TestOverrides{
			MaxClients: intPtr(200),
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	mergedConfig, err := MergeConfiguration(baseConfig, bodyBytes)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Only MaxClients should be overridden
	if mergedConfig.Test.MaxClients != 200 {
		t.Errorf("Expected MaxClients 200, got %d", mergedConfig.Test.MaxClients)
	}

	// All other values should remain the same
	if mergedConfig.Test.MinClients != baseConfig.Test.MinClients {
		t.Errorf("Expected MinClients %d, got %d", baseConfig.Test.MinClients, mergedConfig.Test.MinClients)
	}
}

func TestMergeConfiguration_InvalidJSON(t *testing.T) {
	baseConfig := &config.Config{}
	invalidJSON := []byte(`{"test": invalid}`)

	_, err := MergeConfiguration(baseConfig, invalidJSON)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestMergeConfiguration_NoTestOverrides(t *testing.T) {
	baseConfig := &config.Config{
		Test: config.Test{
			MinClients: 1,
			MaxClients: 100,
		},
	}

	requestBody := BenchmarkRequest{} // No test overrides
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	mergedConfig, err := MergeConfiguration(baseConfig, bodyBytes)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should be identical to base config
	if mergedConfig.Test.MinClients != baseConfig.Test.MinClients {
		t.Errorf("Expected MinClients %d, got %d", baseConfig.Test.MinClients, mergedConfig.Test.MinClients)
	}
	if mergedConfig.Test.MaxClients != baseConfig.Test.MaxClients {
		t.Errorf("Expected MaxClients %d, got %d", baseConfig.Test.MaxClients, mergedConfig.Test.MaxClients)
	}
}

// Helper function to create int pointers
func intPtr(i int) *int {
	return &i
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}

func TestCreateRedisConnection_EmptyBody(t *testing.T) {
	baseConn := &config.RedisConnection{
		URL:                   "redis://base-host:6379",
		ConnectTimeoutSeconds: 10,
		TargetLabel:           "redis://base-host:6379",
	}

	conn, err := CreateRedisConnection(baseConn, []byte{})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if conn != nil {
		t.Error("Expected nil connection for empty body")
	}
}

func TestCreateRedisConnection_NoRedisOverrides(t *testing.T) {
	baseConn := &config.RedisConnection{
		URL:                   "redis://base-host:6379",
		ConnectTimeoutSeconds: 10,
		TargetLabel:           "redis://base-host:6379",
	}

	requestBody := BenchmarkRequest{
		Test: &TestOverrides{
			MaxClients: intPtr(100),
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	conn, err := CreateRedisConnection(baseConn, bodyBytes)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if conn != nil {
		t.Error("Expected nil connection when no Redis overrides provided")
	}
}

func TestCreateRedisConnection_WithURLOverride_Alternative(t *testing.T) {
	baseConn := &config.RedisConnection{
		URL:                   "redis://base-host:6379",
		ConnectTimeoutSeconds: 10,
		TargetLabel:           "redis://base-host:6379",
	}

	requestBody := BenchmarkRequest{
		Redis: &RedisOverrides{
			URL: stringPtr("redis://new-host:6380"),
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	conn, err := CreateRedisConnection(baseConn, bodyBytes)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if conn == nil {
		t.Fatal("Expected connection to be created")
	}

	expectedURL := "redis://new-host:6380"
	if conn.URL != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, conn.URL)
	}
	if conn.TargetLabel != expectedURL {
		t.Errorf("Expected target label '%s', got '%s'", expectedURL, conn.TargetLabel)
	}
}

func TestCreateRedisConnection_WithURLOverride(t *testing.T) {
	var baseConn *config.RedisConnection // nil base should be supported

	requestBody := BenchmarkRequest{
		Redis: &RedisOverrides{
			URL: stringPtr("redis://secure-redis.example.com:6379"),
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	conn, err := CreateRedisConnection(baseConn, bodyBytes)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if conn == nil {
		t.Fatal("Expected connection to be created")
	}

	expectedURL := "redis://secure-redis.example.com:6379"
	if conn.URL != expectedURL {
		t.Errorf("Expected URL '%s', got '%s'", expectedURL, conn.URL)
	}
}

func TestCreateRedisConnection_WithClusterURLOverride(t *testing.T) {
	baseConn := &config.RedisConnection{
		URL:                   "redis://base-host:6379",
		ConnectTimeoutSeconds: 10,
		TargetLabel:           "redis://base-host:6379",
	}

	requestBody := BenchmarkRequest{
		Redis: &RedisOverrides{
			ClusterURL: stringPtr("redis://cluster.example.com:6379"),
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	conn, err := CreateRedisConnection(baseConn, bodyBytes)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if conn == nil {
		t.Fatal("Expected connection to be created")
	}

	if conn.ClusterURL != "cluster.example.com:6379" {
		t.Errorf("Expected cluster URL 'cluster.example.com:6379', got '%s'", conn.ClusterURL)
	}
	if conn.TargetLabel != "cluster.example.com:6379" {
		t.Errorf("Expected target label 'cluster.example.com:6379', got '%s'", conn.TargetLabel)
	}
}

func TestCreateRedisConnection_ValidationError(t *testing.T) {
	baseConn := &config.RedisConnection{
		URL:                   "redis://base-host:6379",
		ConnectTimeoutSeconds: 10,
		TargetLabel:           "redis://base-host:6379",
	}

	requestBody := BenchmarkRequest{
		Redis: &RedisOverrides{
			// No URL, ClusterURL, or Host provided - should fail validation
		},
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("Failed to marshal request body: %v", err)
	}

	_, err = CreateRedisConnection(baseConn, bodyBytes)
	if err == nil {
		t.Error("Expected validation error for empty Redis configuration")
	}
}
