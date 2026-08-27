package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Host, Port, Name, Version       string
	LLMProvider, LLMModel, LLMKey   string
	GeminiAPIKey, GoogleSearchModel string
}

func Load() (Config, error) {
	cfg := Config{
		Host: os.Getenv("HOST"), Port: os.Getenv("PORT"), Name: os.Getenv("NAME"), Version: os.Getenv("VERSION"),
		LLMProvider: strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER"))), LLMModel: strings.TrimSpace(os.Getenv("LLM_MODEL")), LLMKey: strings.TrimSpace(os.Getenv("LLM_KEY")),
		GeminiAPIKey: strings.TrimSpace(os.Getenv("GEMINI_API_KEY")), GoogleSearchModel: strings.TrimSpace(os.Getenv("GOOGLE_SEARCH_MODEL")),
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" {
		return Config{}, fmt.Errorf("HOST, PORT, NAME, and VERSION are required")
	}
	return cfg, nil
}
