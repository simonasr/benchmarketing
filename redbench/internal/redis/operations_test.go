package redis

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockClient is a mock implementation of the Client interface
type MockClient struct {
	mock.Mock
}

func (m *MockClient) Set(ctx context.Context, key string, value string, expiration int32) error {
	args := m.Called(ctx, key, value, expiration)
	return args.Error(0)
}

func (m *MockClient) Get(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *MockClient) MSet(ctx context.Context, kv map[string]string) error {
	args := m.Called(ctx, kv)
	return args.Error(0)
}

func (m *MockClient) MGet(ctx context.Context, keys []string) error {
	args := m.Called(ctx, keys)
	return args.Error(0)
}

func (m *MockClient) HSet(ctx context.Context, key string, fieldValues map[string]string) error {
	args := m.Called(ctx, key, fieldValues)
	return args.Error(0)
}

func (m *MockClient) HMGet(ctx context.Context, key string, fields []string) error {
	args := m.Called(ctx, key, fields)
	return args.Error(0)
}

// (removed) HGet from mock client

func (m *MockClient) ExpireMany(ctx context.Context, keys []string, expiration int32) error {
	args := m.Called(ctx, keys, expiration)
	return args.Error(0)
}

func (m *MockClient) PoolStats() *redis.PoolStats {
	args := m.Called()
	return args.Get(0).(*redis.PoolStats)
}

func (m *MockClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

// MockMetrics is a mock implementation of the metrics functionality
type MockMetrics struct {
	mock.Mock
}

func (m *MockMetrics) ObserveSetDuration(duration float64) {
	m.Called(duration)
}

func (m *MockMetrics) ObserveGetDuration(duration float64) {
	m.Called(duration)
}

func (m *MockMetrics) ObserveMSetDuration(duration float64) {
	m.Called(duration)
}

func (m *MockMetrics) ObserveMGetDuration(duration float64) {
	m.Called(duration)
}

func (m *MockMetrics) ObserveHSetDuration(duration float64) {
	m.Called(duration)
}
func (m *MockMetrics) ObserveHMGetDuration(duration float64) {
	m.Called(duration)
}

// (removed) ObserveHGetDuration

func (m *MockMetrics) ObserveExpireDuration(duration float64) {
	m.Called(duration)
}

func (m *MockMetrics) IncrementSetFailures() {
	m.Called()
}

func (m *MockMetrics) IncrementGetFailures() {
	m.Called()
}

func (m *MockMetrics) IncrementMSetFailures() {
	m.Called()
}

func (m *MockMetrics) IncrementMGetFailures() {
	m.Called()
}

func (m *MockMetrics) IncrementHSetFailures() {
	m.Called()
}
func (m *MockMetrics) IncrementHMGetFailures() {
	m.Called()
}

// (removed) IncrementHGetFailures

func (m *MockMetrics) IncrementExpireFailures() {
	m.Called()
}

func (m *MockMetrics) UpdateRedisPoolStats(stats *redis.PoolStats) {
	m.Called(stats)
}

func (m *MockMetrics) SetStage(clients float64) {
	m.Called(clients)
}

func TestSaveRandomData(t *testing.T) {
	mockClient := new(MockClient)
	mockMetrics := &MockMetrics{}

	ops := NewOperations(mockClient, mockMetrics, false)

	ctx := context.Background()
	expiration := int32(30)
	keySize := 8
	valueSize := 16

	// Success case
	mockClient.On("Set", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string"), expiration).Return(nil)
	mockMetrics.On("ObserveSetDuration", mock.AnythingOfType("float64")).Return()

	key, err := ops.SaveRandomData(ctx, expiration, keySize, valueSize)
	assert.NoError(t, err)
	assert.Equal(t, keySize, len(key))

	mockClient.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)

	// Error case
	mockClient = new(MockClient)
	mockMetrics = &MockMetrics{}
	ops = NewOperations(mockClient, mockMetrics, false)

	expectedErr := errors.New("redis error")
	mockClient.On("Set", ctx, mock.AnythingOfType("string"), mock.AnythingOfType("string"), expiration).Return(expectedErr)
	mockMetrics.On("IncrementSetFailures").Return()

	_, err = ops.SaveRandomData(ctx, expiration, keySize, valueSize)
	assert.Error(t, err)

	mockClient.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestGetData(t *testing.T) {
	mockClient := new(MockClient)
	mockMetrics := &MockMetrics{}

	ops := NewOperations(mockClient, mockMetrics, false)

	ctx := context.Background()
	key := "test-key"

	// Success case
	mockClient.On("Get", ctx, key).Return("test-value", nil)
	mockMetrics.On("ObserveGetDuration", mock.AnythingOfType("float64")).Return()

	err := ops.GetData(ctx, key)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)

	// Error case
	mockClient = new(MockClient)
	mockMetrics = &MockMetrics{}
	ops = NewOperations(mockClient, mockMetrics, false)

	expectedErr := errors.New("redis error")
	mockClient.On("Get", ctx, key).Return("", expectedErr)
	mockMetrics.On("IncrementGetFailures").Return()
	mockMetrics.On("ObserveGetDuration", mock.AnythingOfType("float64")).Return()

	err = ops.GetData(ctx, key)
	assert.Error(t, err)

	mockClient.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestSaveRandomBatchData(t *testing.T) {
	mockClient := new(MockClient)
	mockMetrics := &MockMetrics{}

	ops := NewOperations(mockClient, mockMetrics, false)

	ctx := context.Background()
	expiration := int32(30)
	keySize := 8
	valueSize := 16
	batchSize := 5

	mockClient.On("MSet", ctx, mock.AnythingOfType("map[string]string")).Return(nil)
	mockMetrics.On("ObserveMSetDuration", mock.AnythingOfType("float64")).Return()
	mockClient.On("ExpireMany", ctx, mock.AnythingOfType("[]string"), expiration).Return(nil)
	mockMetrics.On("ObserveExpireDuration", mock.AnythingOfType("float64")).Return()

	keys, err := ops.SaveRandomBatchData(ctx, expiration, keySize, valueSize, batchSize, "{tag}")
	assert.NoError(t, err)
	assert.Equal(t, batchSize, len(keys))

	mockClient.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}

func TestGetBatchData(t *testing.T) {
	mockClient := new(MockClient)
	mockMetrics := &MockMetrics{}

	ops := NewOperations(mockClient, mockMetrics, false)

	ctx := context.Background()
	keys := []string{"a", "b", "c"}

	mockClient.On("MGet", ctx, keys).Return(nil)
	mockMetrics.On("ObserveMGetDuration", mock.AnythingOfType("float64")).Return()

	err := ops.GetBatchData(ctx, keys)
	assert.NoError(t, err)

	mockClient.AssertExpectations(t)
	mockMetrics.AssertExpectations(t)
}
