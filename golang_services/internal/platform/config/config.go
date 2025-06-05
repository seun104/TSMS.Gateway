package config

import (
	"log"
	"strings"

	"github.com/spf13/viper"
)

type AppConfig struct {
	ServerPort     int    `mapstructure:"SERVER_PORT"`
	LogLevel       string `mapstructure:"LOG_LEVEL"`
	PostgresDSN    string `mapstructure:"POSTGRES_DSN"`
	NATSUrl        string `mapstructure:"NATS_URL"`

	UserServiceGRPCPort   int    `mapstructure:"USER_SERVICE_GRPC_PORT"`
	JWTAccessSecret       string `mapstructure:"JWT_ACCESS_SECRET"`
	JWTRefreshSecret      string `mapstructure:"JWT_REFRESH_SECRET"`
	JWTAccessExpiryHours  int    `mapstructure:"JWT_ACCESS_EXPIRY_HOURS"`
	JWTRefreshExpiryHours int    `mapstructure:"JWT_REFRESH_EXPIRY_HOURS"`

	PublicAPIServicePort      int    `mapstructure:"PUBLIC_API_SERVICE_PORT"`
	UserServiceGRPCClientTarget string `mapstructure:"USER_SERVICE_GRPC_CLIENT_TARGET"`

    BillingServiceGRPCPort int    `mapstructure:"BILLING_SERVICE_GRPC_PORT"`
    BillingServiceGRPCClientTarget string `mapstructure:"BILLING_SERVICE_GRPC_CLIENT_TARGET"`

    PhonebookServiceGRPCPort int    `mapstructure:"PHONEBOOK_SERVICE_GRPC_PORT"`
    PhonebookServiceGRPCClientTarget string `mapstructure:"PHONEBOOK_SERVICE_GRPC_CLIENT_TARGET"`
}

// This struct is a generic wrapper. The actual config values are loaded based on the service calling Load.
// It might be better to have service-specific config structs or a map[string]interface{} for AppSpecific.
// For now, AppConfig will grow. Consider refactoring config loading if it becomes too unwieldy.
type GRPCClientConfig struct { // This might not be used if targets are directly in AppConfig
	Target string `mapstructure:"TARGET"` // e.g., "localhost:50051"
}

type ServiceSpecificConfig struct {
	// Example: could hold GRPCPort for the service itself if not globally defined
	// GRPCPort int `mapstructure:"GRPC_PORT"`
	// Add other service-specific non-shared configs here
}


// Config holds all configuration for the application.
// It's a bit of a monolith now; consider splitting if services have very different needs.
type Config struct {
	LogLevel       string `mapstructure:"LOG_LEVEL"`
	PostgresDSN    string `mapstructure:"POSTGRES_DSN"`
	NATSUrl        string `mapstructure:"NATS_URL"` // NATS_URL in yaml

	// User Service Specific (also used by clients)
	UserServiceGRPCPort         int    `mapstructure:"USER_SERVICE_GRPC_PORT"`
	UserServiceGRPCClientTarget string `mapstructure:"USER_SERVICE_GRPC_CLIENT_TARGET"`
	JWTAccessSecret             string `mapstructure:"JWT_ACCESS_SECRET"`
	JWTRefreshSecret            string `mapstructure:"JWT_REFRESH_SECRET"`
	JWTAccessExpiryHours        int    `mapstructure:"JWT_ACCESS_EXPIRY_HOURS"`
	JWTRefreshExpiryHours       int    `mapstructure:"JWT_REFRESH_EXPIRY_HOURS"`

	// Public API Service Specific
	PublicAPIServicePort int `mapstructure:"PUBLIC_API_SERVICE_PORT"`

	// Billing Service Specific (also used by clients)
	BillingServiceGRPCPort         int    `mapstructure:"BILLING_SERVICE_GRPC_PORT"`
	BillingServiceGRPCClientTarget string `mapstructure:"BILLING_SERVICE_GRPC_CLIENT_TARGET"`

	// Phonebook Service Specific (also used by clients)
	PhonebookServiceGRPCPort         int    `mapstructure:"PHONEBOOK_SERVICE_GRPC_PORT"`
	PhonebookServiceGRPCClientTarget string `mapstructure:"PHONEBOOK_SERVICE_GRPC_CLIENT_TARGET"`

	// AppSpecific can hold configurations that are not common or don't have direct fields above.
	// Example: if a service needs a "FOO_API_KEY", it could be APP_FOO_API_KEY -> FooAPIKey in AppSpecific.
	// This requires viper.Unmarshal(&cfg) and then potentially another unmarshal/lookup for service-specific parts.
	// For simplicity with current structure, direct fields are used.
	AppSpecific map[string]string `mapstructure:",remain"` // Captures other keys
}


func Load(serviceName string) (*Config, error) { // serviceName can be used to load serviceName.yaml or for context
	v := viper.New()
	// Base config name (e.g., config.defaults.yaml)
	v.SetConfigName("config.defaults") // Base defaults
	v.SetConfigType("yaml")
	// Standard config paths
	v.AddConfigPath("./configs")          // For running from service cmd dir, e.g., cmd/user_service/
	v.AddConfigPath("../configs")         // For running from service cmd dir like cmd/service/
	v.AddConfigPath("../../configs")       // For running from tests within a service's internal package
	v.AddConfigPath("../../../configs")    // For running from tests deep within a service's internal package
	v.AddConfigPath("./golang_services/configs") // For running from repo root for some tools
	v.AddConfigPath(".")                  // For cases where config might be in current dir (e.g. repo root)


	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
    v.SetEnvPrefix("APP") // APP_LOG_LEVEL, APP_POSTGRES_DSN etc.

	// Set defaults for all known keys
	v.SetDefault("LOG_LEVEL", "info")
    v.SetDefault("POSTGRES_DSN", "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable")
    v.SetDefault("NATS_URL", "nats://localhost:4222")

	v.SetDefault("USER_SERVICE_GRPC_PORT", 50051)
    v.SetDefault("USER_SERVICE_GRPC_CLIENT_TARGET", "localhost:50051")
    v.SetDefault("JWT_ACCESS_SECRET", "access-secret-must-be-overridden-in-prod")
    v.SetDefault("JWT_REFRESH_SECRET", "refresh-secret-must-be-overridden-in-prod")
    v.SetDefault("JWT_ACCESS_EXPIRY_HOURS", 1)
    v.SetDefault("JWT_REFRESH_EXPIRY_HOURS", 720)

    v.SetDefault("PUBLIC_API_SERVICE_PORT", 8080)

    v.SetDefault("BILLING_SERVICE_GRPC_PORT", 50052)
    v.SetDefault("BILLING_SERVICE_GRPC_CLIENT_TARGET", "localhost:50052")

    v.SetDefault("PHONEBOOK_SERVICE_GRPC_PORT", 50051) // Default internal port for Phonebook service
    v.SetDefault("PHONEBOOK_SERVICE_GRPC_CLIENT_TARGET", "localhost:50051") // Default target for clients


	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("Base configuration file ('config.defaults.yaml') not found; using defaults and environment variables.")
		} else {
			return nil, err
		}
	}

	// Optionally, try to merge service-specific config: e.g., user_service.yaml
	// v.SetConfigName(serviceName) // e.g. "user_service.yaml"
	// if err := v.MergeInConfig(); err != nil {
	//     if _, ok := err.(viper.ConfigFileNotFoundError); ok {
	//         log.Printf("Service-specific configuration file '%s.yaml' not found.", serviceName)
	//     } else {
	//          // return nil, err // Decide if this is a fatal error
	//          log.Printf("Warning: Could not merge service-specific config '%s.yaml': %v", serviceName, err)
	//     }
	// }


	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// The AppConfig struct was too broad and becoming a mix of all service configs.
// The new Config struct is also monolithic for now but provides a clearer structure.
// The Load function is simplified to take serviceName for potential future use in loading
// layered configs (e.g., defaults + service-specific overrides), but currently only loads 'config.defaults'.
// The config paths are adjusted to be more flexible.
// The GRPCClientConfig and ServiceSpecificConfig are examples for future refactoring if needed.
	v := viper.New()
	v.SetConfigName(configName)
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)
	v.AddConfigPath(".")
    v.AddConfigPath("../../configs") // For running from cmd/serviceX

	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
    v.SetEnvPrefix("APP")

	v.SetDefault("LOG_LEVEL", "info")
    v.SetDefault("POSTGRES_DSN", "postgres://smsuser:smspassword@localhost:5432/sms_gateway_db?sslmode=disable")
    v.SetDefault("NATS_URL", "nats://localhost:4222")
	v.SetDefault("USER_SERVICE_GRPC_PORT", 50051)
    v.SetDefault("JWT_ACCESS_SECRET", "access-secret-must-be-overridden-in-prod")
    v.SetDefault("JWT_REFRESH_SECRET", "refresh-secret-must-be-overridden-in-prod")
    v.SetDefault("JWT_ACCESS_EXPIRY_HOURS", 1)
    v.SetDefault("JWT_REFRESH_EXPIRY_HOURS", 720)
    v.SetDefault("PUBLIC_API_SERVICE_PORT", 8080)
    v.SetDefault("USER_SERVICE_GRPC_CLIENT_TARGET", "localhost:50051")
    v.SetDefault("BILLING_SERVICE_GRPC_PORT", 50052)
    v.SetDefault("BILLING_SERVICE_GRPC_CLIENT_TARGET", "localhost:50052") // Added Default

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("Configuration file not found; using defaults and environment variables.")
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
