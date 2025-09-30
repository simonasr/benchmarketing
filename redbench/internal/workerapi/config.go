package workerapi

import (
	"encoding/json"
	"fmt"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// BenchmarkRequest represents the request body for starting a benchmark.
// It allows overriding specific benchmark parameters for a single run.
type BenchmarkRequest struct {
	JobID string          `json:"jobId,omitempty"`
	Redis *RedisOverrides `json:"redis,omitempty"`
	Test  *TestOverrides  `json:"test,omitempty"`
}

// RedisOverrides allows specifying Redis target and configuration for the benchmark.
type RedisOverrides struct {
	// URL supports redis:// scheme
	URL        *string `json:"url,omitempty"`
	ClusterURL *string `json:"clusterUrl,omitempty"`
	// Redis configuration overrides
	OperationTimeoutMs *int `json:"operationTimeoutMs,omitempty"`
	Expiration         *int `json:"expiration,omitempty"`
}

// TestOverrides allows overriding specific test configuration values.
type TestOverrides struct {
	MinClients             *int    `json:"minClients,omitempty"`
	MaxClients             *int    `json:"maxClients,omitempty"`
	StageIntervalMs        *int    `json:"stageIntervalMs,omitempty"`
	RequestDelayMs         *int    `json:"requestDelayMs,omitempty"`
	KeySize                *int    `json:"keySize,omitempty"`
	ValueSize              *int    `json:"valueSize,omitempty"`
	Workload               *string `json:"workload,omitempty"`
	BatchSize              *int    `json:"batchSize,omitempty"`
	SameSlotPerClient      *bool   `json:"sameSlotPerClient,omitempty"`
	TagsCount              *int    `json:"tagsCount,omitempty"`
	ZSetBatchSize          *int    `json:"zsetBatchSize,omitempty"`
	ZSetTopK               *int    `json:"zsetTopK,omitempty"`
	ZSetPerTagLeaderboards *int    `json:"zsetPerTagLeaderboards,omitempty"`
	ZSetUnionFanIn         *int    `json:"zsetUnionFanIn,omitempty"`
	ZSetUnionEveryNOps     *int    `json:"zsetUnionEveryNOps,omitempty"`
	ZSetUpdateRatio        *int    `json:"zsetUpdateRatio,omitempty"`
	ZSetScoreMode          *string `json:"zsetScoreMode,omitempty"`
}

// ParseBenchmarkRequest parses the raw request body into a BenchmarkRequest.
func ParseBenchmarkRequest(requestBody []byte) (*BenchmarkRequest, error) {
	if len(requestBody) == 0 {
		return &BenchmarkRequest{}, nil
	}
	var req BenchmarkRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// MergeConfiguration creates a new configuration by merging base config with API request overrides.
// Priority: API Request Body (highest) > Environment Variables (medium) > config.yaml (lowest)
// Note: Environment variables are already processed in config.LoadConfig()
func MergeConfiguration(baseConfig *config.Config, requestBody []byte) (*config.Config, error) {
	// If no request body provided, return a shallow copy of base config as-is.
	// Safe: all fields in config.Config and nested structs are value types or strings
	// (no slices/maps/pointers). If pointer/slice/map fields are added in the future,
	// replace this with an explicit deep copy.
	if len(requestBody) == 0 {
		mergedConfig := &config.Config{}
		*mergedConfig = *baseConfig // copy all fields by value
		return mergedConfig, nil
	}

	// Parse the request body
	var req BenchmarkRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		return nil, err
	}

	return MergeConfigurationFromRequest(baseConfig, &req)
}

// MergeConfigurationFromRequest merges using an already parsed request to avoid duplicate unmarshalling.
func MergeConfigurationFromRequest(baseConfig *config.Config, req *BenchmarkRequest) (*config.Config, error) {
	mergedConfig := &config.Config{}
	// Safe shallow copy (see rationale above). Update to deep copy if mutable reference
	// types are introduced into config.Config in the future.
	*mergedConfig = *baseConfig
	if req == nil {
		return mergedConfig, nil
	}
	// Apply test configuration overrides if provided
	if req.Test != nil {
		applyTestOverrides(&mergedConfig.Test, req.Test)
	}
	// Apply Redis configuration overrides if provided
	if req.Redis != nil {
		applyRedisConfigOverrides(&mergedConfig.Redis, req.Redis)
	}
	return mergedConfig, nil
}

// CreateRedisConnection creates a Redis connection configuration from API request overrides.
// If no Redis configuration is provided in the request, it returns nil (use default connection).
func CreateRedisConnection(baseRedisConn *config.RedisConnection, requestBody []byte) (*config.RedisConnection, error) {
	// If no request body provided, return nil to use default connection
	if len(requestBody) == 0 {
		return nil, nil
	}

	// Parse the request body
	var req BenchmarkRequest
	if err := json.Unmarshal(requestBody, &req); err != nil {
		return nil, fmt.Errorf("parsing request body: %w", err)
	}

	return CreateRedisConnectionFromRequest(baseRedisConn, &req)
}

// CreateRedisConnectionFromRequest creates a Redis connection using an already parsed request.
func CreateRedisConnectionFromRequest(baseRedisConn *config.RedisConnection, req *BenchmarkRequest) (*config.RedisConnection, error) {
	// If no Redis overrides provided, return nil to use default connection
	if req == nil || req.Redis == nil {
		return nil, nil
	}

	// Create a new Redis connection based on the request
	conn := config.NewRedisConnection(baseRedisConn.ConnectTimeoutSeconds)

	// Apply Redis overrides
	if err := applyRedisOverrides(conn, req.Redis); err != nil {
		return nil, fmt.Errorf("applying Redis overrides: %w", err)
	}

	// Set target label using shared method
	conn.SetTargetLabel()

	return conn, nil
}

// applyRedisOverrides applies Redis configuration overrides to the connection.
func applyRedisOverrides(conn *config.RedisConnection, overrides *RedisOverrides) error {
	// Handle URL-based configuration
	if overrides.URL != nil {
		conn.URL = *overrides.URL
		if err := conn.ParseURL(); err != nil {
			return fmt.Errorf("parsing Redis URL: %w", err)
		}
	} else if overrides.ClusterURL != nil {
		conn.ClusterURL = *overrides.ClusterURL
		if err := conn.ParseClusterURL(); err != nil {
			return fmt.Errorf("parsing Redis cluster URL: %w", err)
		}
	}

	// Validate that we have enough information to connect
	if conn.URL == "" && conn.ClusterURL == "" {
		return fmt.Errorf("redis connection requires either url or clusterUrl to be specified")
	}

	return nil
}

// applyTestOverrides applies non-nil override values to the test configuration.
func applyTestOverrides(testConfig *config.Test, overrides *TestOverrides) {
	if overrides.MinClients != nil {
		testConfig.MinClients = *overrides.MinClients
	}
	if overrides.MaxClients != nil {
		testConfig.MaxClients = *overrides.MaxClients
	}
	if overrides.StageIntervalMs != nil {
		testConfig.StageIntervalMs = *overrides.StageIntervalMs
	}
	if overrides.RequestDelayMs != nil {
		testConfig.RequestDelayMs = *overrides.RequestDelayMs
	}
	if overrides.KeySize != nil {
		testConfig.KeySize = *overrides.KeySize
	}
	if overrides.ValueSize != nil {
		testConfig.ValueSize = *overrides.ValueSize
	}
	if overrides.Workload != nil {
		testConfig.Workload = *overrides.Workload
	}
	if overrides.BatchSize != nil {
		testConfig.BatchSize = *overrides.BatchSize
	}
	if overrides.SameSlotPerClient != nil {
		testConfig.SameSlotPerClient = *overrides.SameSlotPerClient
	}
	if overrides.TagsCount != nil {
		testConfig.TagsCount = *overrides.TagsCount
	}
	if overrides.ZSetTopK != nil {
		testConfig.ZSetTopK = *overrides.ZSetTopK
	}
	if overrides.ZSetBatchSize != nil {
		testConfig.ZSetBatchSize = *overrides.ZSetBatchSize
	}
	if overrides.ZSetPerTagLeaderboards != nil {
		testConfig.ZSetPerTagLeaderboards = *overrides.ZSetPerTagLeaderboards
	}
	if overrides.ZSetUnionFanIn != nil {
		testConfig.ZSetUnionFanIn = *overrides.ZSetUnionFanIn
	}
	if overrides.ZSetUnionEveryNOps != nil {
		testConfig.ZSetUnionEveryNOps = *overrides.ZSetUnionEveryNOps
	}
	if overrides.ZSetUpdateRatio != nil {
		testConfig.ZSetUpdateRatio = *overrides.ZSetUpdateRatio
	}
	if overrides.ZSetScoreMode != nil {
		testConfig.ZSetScoreMode = *overrides.ZSetScoreMode
	}
}

// applyRedisConfigOverrides applies non-nil override values to the Redis configuration.
func applyRedisConfigOverrides(redisConfig *config.RedisConfig, overrides *RedisOverrides) {
	if overrides.OperationTimeoutMs != nil {
		redisConfig.OperationTimeoutMs = *overrides.OperationTimeoutMs
	}
	if overrides.Expiration != nil {
		redisConfig.Expiration = int32(*overrides.Expiration)
	}
}
