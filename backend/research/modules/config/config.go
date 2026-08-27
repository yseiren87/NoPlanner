package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host                     string
	Port                     string
	Name                     string
	Version                  string
	DatabaseURL              string
	IntelligenceGRPCAddress  string
	GoogleSearchCostPerQuery float64
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
	if raw := os.Getenv("GOOGLE_SEARCH_COST_PER_QUERY"); raw != "" {
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil || value < 0 {
			return Config{}, fmt.Errorf("GOOGLE_SEARCH_COST_PER_QUERY must be a non-negative number")
		}
		cfg.GoogleSearchCostPerQuery = value
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" || cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("HOST, PORT, NAME, VERSION, and DATABASE_URL are required")
	}
	return cfg, nil
}
