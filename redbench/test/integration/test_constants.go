package integration

import (
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// Test configuration constants to avoid magic numbers
const (
	// Test cycle configuration
	DefaultTestCycles = 3

	// Port ranges for integration tests - organized to prevent conflicts
	// Uses high port numbers (18xxx) to avoid system services
	ControllerPortBase = 18100 // Base range for controllers
	WorkerPortBase     = 18200 // Base range for workers (future use)
	ServicePortBase    = 18300 // Base range for services (future use)

	// Specific test ports - assigned systematically by test category
	MockRedisControllerPort         = 18102 // Mock Redis lifecycle tests
	MockRedisWorkerPort             = 18103
	MockRedisTimeoutControllerPort  = 18104 // Mock Redis timeout tests
	MockRedisTimeoutWorkerPort      = 18105
	RepeatedLifecycleControllerPort = 18120 // Repeated lifecycle tests
	RepeatedLifecycleWorkerPort     = 18121
	ServiceLifecyclePort            = 18125 // Service mode tests
	BasicControllerPort             = 18081 // Basic integration tests
	BasicWorkerPort                 = 18080

	// Test timeouts and delays - carefully chosen based on CI environment testing
	// and actual system behavior observation
	StartupDelay         = 200 * time.Millisecond // HTTP server initialization time
	RegistrationDelay    = 300 * time.Millisecond // Worker registration + initial handshake
	ShutdownDelay        = 200 * time.Millisecond // Graceful shutdown completion
	CycleDelay           = 100 * time.Millisecond // Brief pause between test cycles
	BenchmarkRunDuration = 800 * time.Millisecond // Min time for meaningful benchmark ops
	ServiceRunDuration   = 600 * time.Millisecond // Service mode benchmark duration
	FinalTestDuration    = 400 * time.Millisecond // Final validation runs
	LongRunDuration      = 3 * time.Second        // Extended operations (long timeout tests)

	// Test benchmark configuration - balanced for reliable CI execution
	// and meaningful operation generation
	TestMinClients          = 1   // Always start with single client
	TestMaxClientsSmall     = 2   // For service tests - minimal load
	TestMaxClientsMedium    = 3   // For controller-worker tests - moderate load
	TestStageIntervalFast   = 200 // ms - Quick progression for service tests
	TestStageIntervalNormal = 300 // ms - Standard progression for comprehensive tests
	TestRequestDelayFast    = 10  // ms - High throughput for operation generation
	TestRequestDelayNormal  = 20  // ms - Moderate speed for realistic timing
	TestKeySize             = 8   // Small keys for fast operations
	TestValueSizeSmall      = 12  // Compact values for service tests
	TestValueSizeNormal     = 16  // Standard values for comprehensive tests

	// Test labels and identifiers
	MockRedisLabel   = "mock-redis"
	TestRedisLabel   = "test-redis"
	CycleMarkerKey   = "cycle-marker"
	ServiceCycleKey  = "service-cycle"
	ResetTestKey     = "reset-test"
	LocalhostAddress = "localhost"
	UnknownHostname  = "unknown"

	// Test IP addresses for bind tests
	TestIP1 = "10.1.2.3"
	TestIP2 = "192.168.1.100"
)

// Helper functions for common test configurations

// ConfigureQuickBenchmark applies quick benchmark settings to config
func ConfigureQuickBenchmark(cfg *config.Config) {
	cfg.Test.MinClients = TestMinClients
	cfg.Test.MaxClients = TestMaxClientsSmall
	cfg.Test.StageIntervalMs = TestStageIntervalFast
	cfg.Test.RequestDelayMs = TestRequestDelayNormal
	cfg.Test.KeySize = TestKeySize
	cfg.Test.ValueSize = TestValueSizeSmall
}

// ConfigureNormalBenchmark applies normal benchmark settings to config
func ConfigureNormalBenchmark(cfg *config.Config) {
	cfg.Test.MinClients = TestMinClients
	cfg.Test.MaxClients = TestMaxClientsMedium
	cfg.Test.StageIntervalMs = TestStageIntervalNormal
	cfg.Test.RequestDelayMs = TestRequestDelayFast
	cfg.Test.KeySize = TestKeySize
	cfg.Test.ValueSize = TestValueSizeNormal
}
