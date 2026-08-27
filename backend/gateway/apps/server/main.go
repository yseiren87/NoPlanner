package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"noplanner/backend/gateway/modules/clients"
	"noplanner/backend/gateway/modules/config"
	"noplanner/backend/gateway/modules/jobs"
	"noplanner/backend/gateway/modules/observability"
	"noplanner/backend/gateway/modules/token"
	apiservice "noplanner/backend/gateway/services/api"
	"noplanner/backend/gateway/services/status"
	gatewayv1 "noplanner/backend/proto/dist/golang/gateway/v1"
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

	downstream, err := clients.New(clients.Addresses{User: cfg.UserAddress, Project: cfg.ProjectAddress, Document: cfg.DocumentAddress, Research: cfg.ResearchAddress, Evaluation: cfg.EvaluationAddress, Generation: cfg.GenerationAddress}, cfg.InternalServiceToken)
	if err != nil {
		logger.Error("downstream_configuration_failed", "error", err)
		os.Exit(1)
	}
	defer downstream.Close()
	grpcServer := grpc.NewServer(grpc.ChainUnaryInterceptor(observability.UnaryServer(logger)))
	gatewayv1.RegisterGatewayServiceServer(grpcServer, status.New(cfg.Name, cfg.Version))
	queue, err := jobs.New(cfg.RedisURL)
	if err != nil {
		logger.Error("redis_configuration_failed", "error", err)
		os.Exit(1)
	}
	defer queue.Close()
	apiService := apiservice.New(downstream, token.New(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience), queue)
	workerContext, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	go apiService.RunWorker(workerContext)
	apiHandler := apiService.Handler()
	handler := h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
			return
		}
		apiHandler.ServeHTTP(w, r)
	}), &http2.Server{})
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := http.Serve(listener, handler); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
