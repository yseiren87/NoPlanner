package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	Host                    string
	Port                    string
	Name                    string
	Version                 string
	GoogleOAuthClientID     string
	GoogleOAuthClientSecret string
	GoogleOAuthRedirectURL  string
	JWTSecret               string
	JWTIssuer               string
	JWTAudience             string
	JWTTTL                  time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Host:                    os.Getenv("HOST"),
		Port:                    os.Getenv("PORT"),
		Name:                    os.Getenv("NAME"),
		Version:                 os.Getenv("VERSION"),
		GoogleOAuthClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleOAuthClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		GoogleOAuthRedirectURL:  os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
		JWTSecret:               os.Getenv("JWT_SECRET"),
		JWTIssuer:               os.Getenv("JWT_ISSUER"),
		JWTAudience:             os.Getenv("JWT_AUDIENCE"),
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" {
		return Config{}, fmt.Errorf("HOST, PORT, NAME, and VERSION are required")
	}
	if rawTTL := os.Getenv("JWT_TTL"); rawTTL != "" {
		ttl, err := time.ParseDuration(rawTTL)
		if err != nil || ttl <= 0 {
			return Config{}, fmt.Errorf("JWT_TTL must be a positive duration")
		}
		cfg.JWTTTL = ttl
	}
	return cfg, nil
}
