package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

// AppConfig holds the application configuration
type AppConfig struct {
	ServerPort int    `mapstructure:"SERVER_PORT"`
	LogLevel   string `mapstructure:"LOG_LEVEL"`
	// Add other global configurations here
	// Example:
	// NATSUrl string `mapstructure:"NATS_URL"`
	// PostgresDSN string `mapstructure:"POSTGRES_DSN"` // Or break down DSN components
}

// Load loads the configuration from file and environment variables
func Load(configPath string, configName string) (*AppConfig, error) {
	v := viper.New()

	v.SetConfigName(configName) // e.g., "config"
	v.SetConfigType("yaml")     // or "json", "toml"
	v.AddConfigPath(configPath) // e.g., "./configs", "/etc/appname/"
	v.AddConfigPath(".")        // Optionally look in the working directory

	// Read environment variables
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_")) // Replace . with _ in env var names

	// Set default values (optional)
	v.SetDefault("SERVER_PORT", 8080)
	v.SetDefault("LOG_LEVEL", "info")

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("Configuration file not found at %s/%s; using defaults and environment variables", configPath, configName)
		} else {
			return nil, err
		}
	}

	var cfg AppConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
