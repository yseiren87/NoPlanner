package config

import (
	"fmt"
	"os"
)

type Config struct {
	Host                 string
	Port                 string
	Name                 string
	Version              string
	JWTSecret            string
	JWTIssuer            string
	JWTAudience          string
	RedisURL             string
	UserAddress          string
	ProjectAddress       string
	DocumentAddress      string
	ResearchAddress      string
	EvaluationAddress    string
	GenerationAddress    string
	InternalServiceToken string
}

func Load() (Config, error) {
	cfg := Config{
		Host:                 os.Getenv("HOST"),
		Port:                 os.Getenv("PORT"),
		Name:                 os.Getenv("NAME"),
		Version:              os.Getenv("VERSION"),
		JWTSecret:            os.Getenv("JWT_SECRET"),
		JWTIssuer:            os.Getenv("JWT_ISSUER"),
		JWTAudience:          os.Getenv("JWT_AUDIENCE"),
		RedisURL:             os.Getenv("REDIS_URL"),
		UserAddress:          os.Getenv("USER_GRPC_ADDRESS"),
		ProjectAddress:       os.Getenv("PROJECT_GRPC_ADDRESS"),
		DocumentAddress:      os.Getenv("DOCUMENT_GRPC_ADDRESS"),
		ResearchAddress:      os.Getenv("RESEARCH_GRPC_ADDRESS"),
		EvaluationAddress:    os.Getenv("EVALUATION_GRPC_ADDRESS"),
		GenerationAddress:    os.Getenv("GENERATION_GRPC_ADDRESS"),
		InternalServiceToken: os.Getenv("INTERNAL_SERVICE_TOKEN"),
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" || cfg.JWTSecret == "" || cfg.JWTIssuer == "" || cfg.JWTAudience == "" || cfg.InternalServiceToken == "" {
		return Config{}, fmt.Errorf("HOST, PORT, NAME, VERSION, JWT_SECRET, JWT_ISSUER, and JWT_AUDIENCE are required")
	}
	return cfg, nil
}
