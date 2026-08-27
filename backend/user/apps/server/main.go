package main

import (
	"log/slog"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	userv1 "noplanner/backend/proto/dist/golang/user/v1"
	"noplanner/backend/user/domains/authtransaction"
	"noplanner/backend/user/modules/config"
	"noplanner/backend/user/modules/googleoauth"
	"noplanner/backend/user/modules/observability"
	"noplanner/backend/user/modules/token"
	"noplanner/backend/user/services/account"
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

	tokenManager := token.New(cfg.JWTSecret, cfg.JWTIssuer, cfg.JWTAudience, cfg.JWTTTL)
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(
		observability.UnaryServer(logger),
		tokenManager.UnaryServer(userv1.UserService_GetCurrentUser_FullMethodName),
	))
	googleClient := googleoauth.New(cfg.GoogleOAuthClientID, cfg.GoogleOAuthClientSecret, cfg.GoogleOAuthRedirectURL)
	transactions := authtransaction.NewStore(10 * time.Minute)
	userv1.RegisterUserServiceServer(server, account.New(cfg.Name, cfg.Version, transactions, googleClient, tokenManager))
	logger.Info("grpc_server_started", "address", listener.Addr().String())
	if err := server.Serve(listener); err != nil {
		logger.Error("grpc_server_failed", "error", err)
		os.Exit(1)
	}
}
