package main

import (
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	"noplanner/backend/intelligence/modules/config"
	"noplanner/backend/intelligence/modules/auth"
	"noplanner/backend/intelligence/modules/provider"
	intelligenceservice "noplanner/backend/intelligence/services/intelligence"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration_failed", "error", err)
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		logger.Error("listener_failed", "error", err)
		os.Exit(1)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(auth.UnaryServer(os.Getenv("INTERNAL_SERVICE_TOKEN"), intelligencev1.IntelligenceService_GetStatus_FullMethodName)))
	client := provider.New(cfg.LLMProvider, cfg.LLMModel, cfg.LLMKey, cfg.GeminiAPIKey, cfg.GoogleSearchModel)
	intelligencev1.RegisterIntelligenceServiceServer(server, intelligenceservice.New(client, cfg.Name, cfg.Version))
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
