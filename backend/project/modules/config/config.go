package config

import (
	"fmt"
	"os"
)

type Config struct {
	Host        string
	Port        string
	Name        string
	Version     string
	DatabaseURL string
	JWTSecret   string
	JWTIssuer   string
	JWTAudience string
}

func Load() (Config, error) {
	cfg := Config{
		Host:        os.Getenv("HOST"),
		Port:        os.Getenv("PORT"),
		Name:        os.Getenv("NAME"),
		Version:     os.Getenv("VERSION"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		JWTIssuer:   os.Getenv("JWT_ISSUER"),
		JWTAudience: os.Getenv("JWT_AUDIENCE"),
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" || cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("HOST, PORT, NAME, VERSION, and DATABASE_URL are required")
	}
	return cfg, nil
}
