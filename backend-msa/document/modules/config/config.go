package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Host                    string
	Port                    string
	Name                    string
	Version                 string
	DatabaseURL             string
	ObjectStorageEndpoint   string
	ObjectStorageRegion     string
	ObjectStorageBucket     string
	ObjectStorageAccessKey  string
	ObjectStorageSecretKey  string
	ObjectStoragePathStyle  bool
	IntelligenceGRPCAddress string
}

func Load() (Config, error) {
	cfg := Config{
		Host:                    os.Getenv("HOST"),
		Port:                    os.Getenv("PORT"),
		Name:                    os.Getenv("NAME"),
		Version:                 os.Getenv("VERSION"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		ObjectStorageEndpoint:   os.Getenv("OBJECT_STORAGE_ENDPOINT"),
		ObjectStorageRegion:     os.Getenv("OBJECT_STORAGE_REGION"),
		ObjectStorageBucket:     os.Getenv("OBJECT_STORAGE_BUCKET"),
		ObjectStorageAccessKey:  os.Getenv("OBJECT_STORAGE_ACCESS_KEY"),
		ObjectStorageSecretKey:  os.Getenv("OBJECT_STORAGE_SECRET_KEY"),
		IntelligenceGRPCAddress: os.Getenv("INTELLIGENCE_GRPC_ADDRESS"),
	}
	if raw := os.Getenv("OBJECT_STORAGE_PATH_STYLE"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("OBJECT_STORAGE_PATH_STYLE must be a boolean")
		}
		cfg.ObjectStoragePathStyle = value
	}
	if cfg.Host == "" || cfg.Port == "" || cfg.Name == "" || cfg.Version == "" || cfg.DatabaseURL == "" ||
		cfg.ObjectStorageEndpoint == "" || cfg.ObjectStorageRegion == "" || cfg.ObjectStorageBucket == "" ||
		cfg.ObjectStorageAccessKey == "" || cfg.ObjectStorageSecretKey == "" {
		return Config{}, fmt.Errorf("service, database, and object storage configuration are required")
	}
	return cfg, nil
}
