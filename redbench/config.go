package main

import (
	"os"

	"gopkg.in/yaml.v2"
)

type Config struct {
	MetricsPort int         `yaml:"metricsPort"`
	Debug       bool        `yaml:"debug"`
	Redis       RedisConfig `yaml:"redis"`
	Test        Test        `yaml:"test"`
}

type RedisConfig struct {
	Expiration         int32 `yaml:"expirationS"`
	OperationTimeoutMs int   `yaml:"operationTimeoutMs"`
}

type Test struct {
	MinClients     int `yaml:"minClients"`
	MaxClients     int `yaml:"maxClients"`
	StageIntervalS int `yaml:"stageIntervalS"`
	RequestDelayMs int `yaml:"requestDelayMs"`
	KeySize        int `yaml:"keySize"`
	ValueSize      int `yaml:"valueSize"`
}

func (c *Config) loadConfig(path string) {
	f, err := os.ReadFile(path)
	if err != nil {
		panic("os.ReadFile failed: " + err.Error())
	}

	err = yaml.Unmarshal(f, c)
	if err != nil {
		panic("yaml.Unmarshal failed: " + err.Error())
	}
}
