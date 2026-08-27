package config

import (
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Host                string
	Port                string
	Name                string
	Version             string
	DatabaseURL         string
	IntelligenceAddress string
	EvaluationAddress   string
	RepositoryRoots     []string
	Connectors          map[string]ConnectorConfig
}
type ConnectorConfig struct{ Endpoint, Token string }

func Load() (Config, error) {
	cfg := Config{
		Host:                os.Getenv("HOST"),
		Port:                os.Getenv("PORT"),
		Name:                os.Getenv("NAME"),
		Version:             os.Getenv("VERSION"),
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		IntelligenceAddress: os.Getenv("INTELLIGENCE_GRPC_ADDRESS"),
		EvaluationAddress:   os.Getenv("EVALUATION_GRPC_ADDRESS"),
		RepositoryRoots:     filepath.SplitList(os.Getenv("REPOSITORY_ALLOWED_ROOTS")),
		Connectors: map[string]ConnectorConfig{
			"drive": {os.Getenv("DRIVE_CONNECTOR_URL"), os.Getenv("DRIVE_CONNECTOR_KEY")}, "notion": {os.Getenv("NOTION_CONNECTOR_URL"), os.Getenv("NOTION_CONNECTOR_KEY")}, "confluence": {os.Getenv("CONFLUENCE_CONNECTOR_URL"), os.Getenv("CONFLUENCE_CONNECTOR_KEY")}, "github": {os.Getenv("GITHUB_CONNECTOR_URL"), os.Getenv("GITHUB_CONNECTOR_KEY")}, "jira": {os.Getenv("JIRA_CONNECTOR_URL"), os.Getenv("JIRA_CONNECTOR_KEY")}, "slack": {os.Getenv("SLACK_CONNECTOR_URL"), os.Getenv("SLACK_CONNECTOR_KEY")},
		},
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" || cfg.DatabaseURL == "" || cfg.IntelligenceAddress == "" || cfg.EvaluationAddress == "" || len(cfg.RepositoryRoots) == 0 {
		return Config{}, fmt.Errorf("HOST, PORT, NAME, VERSION, DATABASE_URL, INTELLIGENCE_GRPC_ADDRESS, EVALUATION_GRPC_ADDRESS, and REPOSITORY_ALLOWED_ROOTS are required")
	}
	return cfg, nil
}
