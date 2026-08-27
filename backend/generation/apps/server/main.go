package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	design "noplanner/backend/generation/domains/design"
	implementation "noplanner/backend/generation/domains/implementation"
	planning "noplanner/backend/generation/domains/planning"
	domain "noplanner/backend/generation/domains/report"
	"noplanner/backend/generation/modules/auth"
	"noplanner/backend/generation/modules/config"
	"noplanner/backend/generation/modules/connectors"
	"noplanner/backend/generation/modules/observability"
	"noplanner/backend/generation/modules/uivalidator"
	reportservice "noplanner/backend/generation/services/report"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
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

	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		logger.Error("listener_failed", "error", err)
		os.Exit(1)
	}
	store, err := domain.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	implementationStore, err := implementation.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("implementation_database_failed", "error", err)
		os.Exit(1)
	}
	defer implementationStore.Close()
	planningStore, err := planning.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("planning_database_failed", "error", err)
		os.Exit(1)
	}
	defer planningStore.Close()
	designStore, err := design.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("design_database_failed", "error", err)
		os.Exit(1)
	}
	defer designStore.Close()
	var intelligence intelligencev1.IntelligenceServiceClient
	if cfg.IntelligenceAddress != "" {
		conn, e := grpc.NewClient(cfg.IntelligenceAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(auth.UnaryClient(os.Getenv("INTERNAL_SERVICE_TOKEN"))))
		if e != nil {
			logger.Error("intelligence_connection_failed", "error", e)
			os.Exit(1)
		}
		defer conn.Close()
		intelligence = intelligencev1.NewIntelligenceServiceClient(conn)
	}
	var evaluation evaluationv1.EvaluationServiceClient
	if cfg.EvaluationAddress != "" {
		conn, e := grpc.NewClient(cfg.EvaluationAddress, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(auth.UnaryClient(os.Getenv("INTERNAL_SERVICE_TOKEN"))))
		if e != nil {
			logger.Error("evaluation_connection_failed", "error", e)
			os.Exit(1)
		}
		defer conn.Close()
		evaluation = evaluationv1.NewEvaluationServiceClient(conn)
	}

	server := grpc.NewServer(grpc.ChainUnaryInterceptor(observability.UnaryServer(logger), auth.UnaryServer(os.Getenv("INTERNAL_SERVICE_TOKEN"), generationv1.GenerationService_GetStatus_FullMethodName)))
	connectorRegistry := connectors.New([]connectors.Config{
		{Name: "drive", Endpoint: cfg.Connectors["drive"].Endpoint, Token: cfg.Connectors["drive"].Token, Capabilities: []connectors.Capability{connectors.ReadDocuments}},
		{Name: "notion", Endpoint: cfg.Connectors["notion"].Endpoint, Token: cfg.Connectors["notion"].Token, Capabilities: []connectors.Capability{connectors.ReadDocuments}},
		{Name: "confluence", Endpoint: cfg.Connectors["confluence"].Endpoint, Token: cfg.Connectors["confluence"].Token, Capabilities: []connectors.Capability{connectors.ReadDocuments}},
		{Name: "github", Endpoint: cfg.Connectors["github"].Endpoint, Token: cfg.Connectors["github"].Token, Capabilities: []connectors.Capability{connectors.WriteWorkItem}},
		{Name: "jira", Endpoint: cfg.Connectors["jira"].Endpoint, Token: cfg.Connectors["jira"].Token, Capabilities: []connectors.Capability{connectors.WriteWorkItem}},
		{Name: "slack", Endpoint: cfg.Connectors["slack"].Endpoint, Token: cfg.Connectors["slack"].Token, Capabilities: []connectors.Capability{connectors.SendMessage}},
	})
	reportServer := reportservice.NewWithImplementation(store, implementationStore, planningStore, designStore, connectorRegistry, intelligence, evaluation, cfg.RepositoryRoots, cfg.Name, cfg.Version)
	reportServer.SetUIValidator(uivalidator.New())
	generationv1.RegisterGenerationServiceServer(server, reportServer)
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
