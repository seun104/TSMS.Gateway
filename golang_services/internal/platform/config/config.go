package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

// AppConfig holds the application configuration
type AppConfig struct {
	ServerPort     int    `mapstructure:"SERVER_PORT"` // General server port, might be deprecated if services define their own
	LogLevel       string `mapstructure:"LOG_LEVEL"`
	PostgresDSN    string `mapstructure:"POSTGRES_DSN"`
	NATSUrl        string `mapstructure:"NATS_URL"`

	// User Service specific
	UserServiceGRPCPort   int    `mapstructure:"USER_SERVICE_GRPC_PORT"`
	JWTAccessSecret       string `mapstructure:"JWT_ACCESS_SECRET"`
	JWTRefreshSecret      string `mapstructure:"JWT_REFRESH_SECRET"`
	JWTAccessExpiryHours  int    `mapstructure:"JWT_ACCESS_EXPIRY_HOURS"`
	JWTRefreshExpiryHours int    `mapstructure:"JWT_REFRESH_EXPIRY_HOURS"`

	// Public API Service specific
	PublicAPIServicePort      int    `mapstructure:"PUBLIC_API_SERVICE_PORT"`
	UserServiceGRPCClientTarget string `mapstructure:"USER_SERVICE_GRPC_CLIENT_TARGET"`
}

// Load loads the configuration from file and environment variables
func Load(configPath string, configName string) (*AppConfig, error) {
	v := viper.New()

	v.SetConfigName(configName) // e.g., "config"
	v.SetConfigType("yaml")     // or "json", "toml"
	v.AddConfigPath(configPath) // e.g., "./configs", "/etc/appname/"
	v.AddConfigPath(".")        // Optionally look in the working directory
	v.AddConfigPath("../../configs") // For running from cmd/serviceX (relative path)

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_")) // Allow . and - in env vars by replacing with _
    v.SetEnvPrefix("APP") // Example: APP_SERVER_PORT, APP_LOG_LEVEL

	// Set default values
	v.SetDefault("SERVER_PORT", 8000) // Generic default, might not be used
	v.SetDefault("LOG_LEVEL", "info")
    v.SetDefault("POSTGRES_DSN", "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable")
    v.SetDefault("NATS_URL", "nats://localhost:4222")

    // User Service Defaults
	v.SetDefault("USER_SERVICE_GRPC_PORT", 50051)
    v.SetDefault("JWT_ACCESS_SECRET", "access-secret-must-be-overridden-in-prod")
    v.SetDefault("JWT_REFRESH_SECRET", "refresh-secret-must-be-overridden-in-prod")
    v.SetDefault("JWT_ACCESS_EXPIRY_HOURS", 1)
    v.SetDefault("JWT_REFRESH_EXPIRY_HOURS", 720) // 30 days

    // Public API Service Defaults
    v.SetDefault("PUBLIC_API_SERVICE_PORT", 8080)
    v.SetDefault("USER_SERVICE_GRPC_CLIENT_TARGET", "localhost:50051")


	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("Configuration file not found (searched paths including %s/%s); using defaults and environment variables.", configPath, configName)
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
