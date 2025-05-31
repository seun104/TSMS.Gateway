module github.com/aradsms/golang_services

go 1.21

require (
	github.com/go-chi/chi/v5 v5.0.10 // Example version for Chi
	github.com/golang-jwt/jwt/v5 v5.2.0 // For JWT handling
	github.com/google/uuid v1.6.0 // For UUID generation
	github.com/jackc/pgx/v5 v5.5.2 // For PostgreSQL
	github.com/spf13/viper v1.18.2
	golang.org/x/crypto v0.18.0 // For bcrypt and sha3
	google.golang.org/grpc v1.60.1 // For gRPC
	google.golang.org/protobuf v1.32.0 // For Protocol Buffers
)

// Add common indirect dependencies that might be needed by the above
// 'go mod tidy' will populate this section correctly later.
// For example, pgxpool might bring in jackc/puddle/v2
// Viper brings in many indirect dependencies.
// grpc brings in parts of golang.org/x/net, golang.org/x/text, etc.
