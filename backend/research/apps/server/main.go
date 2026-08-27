package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	researchv1 "noplanner/backend/proto/dist/golang/research/v1"
	"noplanner/backend/research/domains/investigation"
	"noplanner/backend/research/modules/config"
	"noplanner/backend/research/modules/observability"
	"noplanner/backend/research/modules/auth"
	webclient "noplanner/backend/research/modules/web"
	"noplanner/backend/research/services/investigate"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration_failed", "error", err)
		os.Exit(1)
	}
	logger = logger.With("service", cfg.Name, "version", cfg.Version)

	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		logger.Error("listener_failed", "error", err)
		os.Exit(1)
	}
	store, err := investigation.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	var intelligenceConnection *grpc.ClientConn
	searchConfigured := cfg.IntelligenceGRPCAddress != ""
	if searchConfigured {
		intelligenceConnection, err = grpc.NewClient(cfg.IntelligenceGRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(auth.UnaryClient(os.Getenv("INTERNAL_SERVICE_TOKEN"))))
		if err != nil {
			logger.Error("intelligence_connection_failed", "error", err)
			os.Exit(1)
		}
		defer intelligenceConnection.Close()
	}
	searcher := webclient.New(intelligenceConnection)

	server := grpc.NewServer(grpc.ChainUnaryInterceptor(observability.UnaryServer(logger), auth.UnaryServer(os.Getenv("INTERNAL_SERVICE_TOKEN"), researchv1.ResearchService_GetStatus_FullMethodName)))
	researchv1.RegisterResearchServiceServer(server, investigate.New(store, searcher, searchConfigured, cfg.GoogleSearchCostPerQuery, cfg.Name, cfg.Version))
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
