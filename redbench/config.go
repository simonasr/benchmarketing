package main

import (
	"os"

	"gopkg.in/yaml.v2"
	"reflect"
	"strconv"
	"strings"
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

	// Override Test fields from ENV variables
	testVal := reflect.ValueOf(&c.Test).Elem()
	testType := testVal.Type()
	for i := 0; i < testVal.NumField(); i++ {
		field := testType.Field(i)
		fieldName := field.Name
		// ENV var: TEST_<UPPERCASE_FIELDNAME>
		envName := "TEST_" + toEnvName(fieldName)
		if val, ok := os.LookupEnv(envName); ok {
			switch testVal.Field(i).Kind() {
			case reflect.Int, reflect.Int32, reflect.Int64:
				if intVal, err := strconv.Atoi(val); err == nil {
					testVal.Field(i).SetInt(int64(intVal))
				}
				// Add more types as needed
			}
		}
	}
}

// toEnvName converts CamelCase to upper snake case (e.g., MinClients -> MINCLIENTS)
func toEnvName(s string) string {
	res := ""
	for i, c := range s {
		if i > 0 && c >= 'A' && c <= 'Z' {
			res += "_"
		}
		res += string(c)
	}
	return strings.ToUpper(res)
}
