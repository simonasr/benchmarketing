package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomString(t *testing.T) {
	// Test different lengths
	lengths := []int{0, 1, 5, 10, 20, 100}
	
	for _, length := range lengths {
		result := RandomString(length)
		assert.Equal(t, length, len(result), "Generated string length should match requested length")
	}
	
	// Test uniqueness (probabilistic)
	if testing.Short() {
		t.Skip("Skipping uniqueness test in short mode")
	}
	
	// Generate 100 strings of length 10 and check they're not all the same
	results := make(map[string]bool)
	for i := 0; i < 100; i++ {
		str := RandomString(10)
		results[str] = true
	}
	
	// We should have multiple unique strings (very high probability)
	assert.Greater(t, len(results), 1, "Multiple random strings should be unique")
}