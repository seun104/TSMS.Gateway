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
    BillingServiceGRPCClientTarget string `mapstructure:"BILLING_SERVICE_GRPC_CLIENT_TARGET"` // Added
}

func Load(configPath string, configName string) (*AppConfig, error) {
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
