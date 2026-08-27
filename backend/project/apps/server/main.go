package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	projectdomain "noplanner/backend/project/domains/project"
	"noplanner/backend/project/modules/config"
	"noplanner/backend/project/modules/observability"
	"noplanner/backend/project/modules/token"
	projectservice "noplanner/backend/project/services/project"
	projectv1 "noplanner/backend/proto/dist/golang/project/v1"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration_failed", "error", err)
		os.Exit(1)
	}
	logger = logger.With("service", cfg.Name, "version", cfg.Version)
	store, err := projectdomain.NewPostgreSQLStore(context.Background(), cfg.DatabaseURL)
	if err != nil {
		logger.Error("database_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Host, cfg.Port))
	if err != nil {
		logger.Error("listener_failed", "error", err)
		os.Exit(1)
	}

	verifier := token.New(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience)
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(
		observability.UnaryServer(logger),
		verifier.UnaryServer(projectv1.ProjectService_GetStatus_FullMethodName),
	))
	projectv1.RegisterProjectServiceServer(server, projectservice.New(store, cfg.Name, cfg.Version))
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
