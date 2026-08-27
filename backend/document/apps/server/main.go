package main

import (
	"context"
	"log/slog"
	"net"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	documentdomain "noplanner/backend/document/domains/document"
	"noplanner/backend/document/modules/blobstore"
	"noplanner/backend/document/modules/config"
	"noplanner/backend/document/modules/llm"
	"noplanner/backend/document/modules/observability"
	"noplanner/backend/document/modules/auth"
	"noplanner/backend/document/modules/webfetch"
	documentservice "noplanner/backend/document/services/document"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
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
	ctx := context.Background()
	store, err := documentdomain.NewPostgreSQLStore(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	blobs, err := blobstore.NewS3(ctx, cfg.ObjectStorageEndpoint, cfg.ObjectStorageRegion, cfg.ObjectStorageBucket, cfg.ObjectStorageAccessKey, cfg.ObjectStorageSecretKey, cfg.ObjectStoragePathStyle)
	if err != nil {
		logger.Error("object_storage_failed", "error", err)
		os.Exit(1)
	}

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

	server := grpc.NewServer(grpc.ChainUnaryInterceptor(observability.UnaryServer(logger), auth.UnaryServer(os.Getenv("INTERNAL_SERVICE_TOKEN"), documentv1.DocumentService_GetStatus_FullMethodName)))
	documentv1.RegisterDocumentServiceServer(server, documentservice.New(store, blobs, webfetch.New(), llm.New(intelligenceClient), cfg.Name, cfg.Version))
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
