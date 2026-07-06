package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"
)

// ServiceConfig holds configuration for an external service
type ServiceConfig struct {
	Host string
	Port int
}

// URL returns the full URL for the service with the given path
func (s ServiceConfig) URL(path string) string {
	return fmt.Sprintf("http://%s:%d%s", s.Host, s.Port, path)
}

// Config holds all application configuration
type Config struct {
	// Server settings
	ServerHost string
	ServerPort int

	// Worker settings
	MaxConcurrentSims int
	RequestTimeout    int // seconds
	RetryAttempts     int
	RetryBaseDelay    int // milliseconds

	// External services
	PVService    ServiceConfig
	BUEMService  ServiceConfig
	IgnisService ServiceConfig

	// Callback settings
	CallbackHost   string
	CallbackPort   int
	CallbackSecret string
}

var (
	appConfig *Config
	once      sync.Once
)

// GetConfig returns the singleton configuration instance
func GetConfig() *Config {
	once.Do(func() {
		appConfig = loadConfig()
	})
	return appConfig
}

func loadConfig() *Config {
	return &Config{
		// Server settings
		ServerHost: getEnvString("SERVER_HOST", "0.0.0.0"),
		ServerPort: getEnvInt("SERVER_PORT", 8081),

		// Worker settings - control concurrent simulations
		MaxConcurrentSims: getEnvInt("MAX_CONCURRENT_SIMS", 4),
		RequestTimeout:    getEnvInt("REQUEST_TIMEOUT", 0),    // 0 = no timeout; large models (750+ buildings) need unlimited time
		RetryAttempts:     getEnvInt("RETRY_ATTEMPTS", 0),     // 0 = no retries; retries restart PV simulation from scratch
		RetryBaseDelay:    getEnvInt("RETRY_BASE_DELAY", 1000), // 1 second

		// External services - defaults to Docker network IPs for backward compatibility
		PVService: ServiceConfig{
			Host: getEnvString("PV_SERVICE_HOST", "172.20.0.3"),
			Port: getEnvInt("PV_SERVICE_PORT", 8082),
		},
		BUEMService: ServiceConfig{
			Host: getEnvString("BUEM_SERVICE_HOST", "172.20.0.7"),
			Port: getEnvInt("BUEM_SERVICE_PORT", 5000),
		},
		IgnisService: ServiceConfig{
			Host: getEnvString("IGNIS_SERVICE_HOST", "172.20.0.8"),
			Port: getEnvInt("IGNIS_SERVICE_PORT", 8090),
		},

		// Callback settings
		CallbackHost:   getEnvString("CALLBACK_HOST", "host.docker.internal"),
		CallbackPort:   getEnvInt("CALLBACK_PORT", 3001),
		CallbackSecret: getEnvString("CALLBACK_SECRET", ""),
	}
}

// ServerAddr returns the server listen address
func (c *Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.ServerHost, c.ServerPort)
}

// GetTechServiceConfig returns the service config for a given tech name
func (c *Config) GetTechServiceConfig(techName string) (ServiceConfig, bool) {
	switch techName {
	case "pv_supply":
		return c.PVService, true
	default:
		return ServiceConfig{}, false
	}
}

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
