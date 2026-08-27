package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	domain "noplanner/backend/evaluation/domains/assessment"
	qualitydomain "noplanner/backend/evaluation/domains/quality"
	revisiondomain "noplanner/backend/evaluation/domains/revision"
	"noplanner/backend/evaluation/modules/config"
	"noplanner/backend/evaluation/modules/llm"
	"noplanner/backend/evaluation/modules/observability"
	"noplanner/backend/evaluation/modules/auth"
	assessmentservice "noplanner/backend/evaluation/services/assessment"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	intelligencev1 "noplanner/backend/proto/dist/golang/intelligence/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration_failed", "error", err)
		os.Exit(1)
	}
	logger = logger.With("service", cfg.Name, "version", cfg.Version)
	store, err := domain.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	revisionStore, err := revisiondomain.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("revision_database_failed", "error", err)
		os.Exit(1)
	}
	defer revisionStore.Close()
	qualityStore, err := qualitydomain.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("quality_database_failed", "error", err)
		os.Exit(1)
	}
	defer qualityStore.Close()

	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		logger.Error("listener_failed", "error", err)
		os.Exit(1)
	}
	var intelligenceClient intelligencev1.IntelligenceServiceClient
	if cfg.IntelligenceGRPCAddress != "" {
		connection, dialErr := grpc.NewClient(cfg.IntelligenceGRPCAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(auth.UnaryClient(os.Getenv("INTERNAL_SERVICE_TOKEN"))))
		if dialErr != nil {
			logger.Error("intelligence_connection_failed", "error", dialErr)
			os.Exit(1)
		}
		defer connection.Close()
		intelligenceClient = intelligencev1.NewIntelligenceServiceClient(connection)
	}

	server := grpc.NewServer(grpc.ChainUnaryInterceptor(observability.UnaryServer(logger), auth.UnaryServer(os.Getenv("INTERNAL_SERVICE_TOKEN"), evaluationv1.EvaluationService_GetStatus_FullMethodName)))
	evaluationv1.RegisterEvaluationServiceServer(server, assessmentservice.NewWithQuality(store, revisionStore, qualityStore, llm.New(intelligenceClient), cfg.Name, cfg.Version))
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
