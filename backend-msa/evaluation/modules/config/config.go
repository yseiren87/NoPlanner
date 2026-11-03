package config

import (
	"fmt"
	"os"
)

type Config struct {
	Host                    string
	Port                    string
	Name                    string
	Version                 string
	DatabaseURL             string
	IntelligenceGRPCAddress string
}

func Load() (Config, error) {
	cfg := Config{
		Host:                    os.Getenv("HOST"),
		Port:                    os.Getenv("PORT"),
		Name:                    os.Getenv("NAME"),
		Version:                 os.Getenv("VERSION"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		IntelligenceGRPCAddress: os.Getenv("INTELLIGENCE_GRPC_ADDRESS"),
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" || cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("service and database configuration are required")
	}
	return cfg, nil
}
