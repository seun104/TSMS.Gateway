module github.com/aradsms/golang_services

go 1.21

// Minimal requirements for now, Viper will be added by 'go mod tidy'
// when a service actually imports and uses it.
// For now, we can pre-add it if we know the config package will be used.
require github.com/spf13/viper v1.18.2

// Indirect dependencies will be filled by 'go mod tidy'
