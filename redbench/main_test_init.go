package main

import (
	"fmt"
	"os"
)

func init() {
	// Set test environment variables
	if os.Getenv("REDIS_HOST") == "" && os.Getenv("REDIS_CLUSTER_ADDRESS") == "" {
		os.Setenv("REDIS_HOST", "test-host")
		fmt.Println("Setting REDIS_HOST to test-host")
	} else {
		fmt.Println("REDIS_HOST already set to:", os.Getenv("REDIS_HOST"))
	}
}
