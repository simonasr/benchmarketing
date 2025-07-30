package main

import "os"

func init() {
	// Set test environment variables
	if os.Getenv("REDIS_HOST") == "" && os.Getenv("REDIS_CLUSTER_ADDRESS") == "" {
		os.Setenv("REDIS_HOST", "test-host")
	}
}
