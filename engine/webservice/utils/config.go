package utils

import (
	"fmt"
	"os"
)

// Config represents the configuration of the web service server
type Config struct {
	Domain string `json:"domain"`
	Port   string `json:"port"`
}

// Load initialize the config
func (config *Config) Load() {
	config.Domain = os.Getenv("DOMAIN")
	config.Port = os.Getenv("PORT")
}

// Address returns a address string like "localhost:port"
func (config *Config) Address() string {
	return config.Domain + ":" + config.Port
}

// String returns a string which describes the attributes of Config
func (config Config) String() string {
	return fmt.Sprintf("Domain: %s, Port: %s", config.Domain, config.Port)
}
