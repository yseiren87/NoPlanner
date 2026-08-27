package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	commonv1 "noplanner/backend/proto/dist/golang/common/v1"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	gatewayv1 "noplanner/backend/proto/dist/golang/gateway/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	projectv1 "noplanner/backend/proto/dist/golang/project/v1"
	researchv1 "noplanner/backend/proto/dist/golang/research/v1"
	userv1 "noplanner/backend/proto/dist/golang/user/v1"
)

type statusClient interface {
	GetStatus(context.Context, *commonv1.StatusRequest, ...grpc.CallOption) (*commonv1.StatusResponse, error)
}

type target struct {
	name   string
	addr   string
	client func(grpc.ClientConnInterface) statusClient
}

func main() {
	targets := []target{
		{name: "gateway", addr: "127.0.0.1:8080", client: func(conn grpc.ClientConnInterface) statusClient { return gatewayv1.NewGatewayServiceClient(conn) }},
		{name: "user", addr: "127.0.0.1:8081", client: func(conn grpc.ClientConnInterface) statusClient { return userv1.NewUserServiceClient(conn) }},
		{name: "project", addr: "127.0.0.1:8082", client: func(conn grpc.ClientConnInterface) statusClient { return projectv1.NewProjectServiceClient(conn) }},
		{name: "document", addr: "127.0.0.1:8083", client: func(conn grpc.ClientConnInterface) statusClient { return documentv1.NewDocumentServiceClient(conn) }},
		{name: "research", addr: "127.0.0.1:8084", client: func(conn grpc.ClientConnInterface) statusClient { return researchv1.NewResearchServiceClient(conn) }},
		{name: "evaluation", addr: "127.0.0.1:8085", client: func(conn grpc.ClientConnInterface) statusClient { return evaluationv1.NewEvaluationServiceClient(conn) }},
		{name: "generation", addr: "127.0.0.1:8086", client: func(conn grpc.ClientConnInterface) statusClient { return generationv1.NewGenerationServiceClient(conn) }},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, target := range targets {
		conn, err := grpc.NewClient(target.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s connection: %v\n", target.name, err)
			os.Exit(1)
		}
		response, err := target.client(conn).GetStatus(ctx, &commonv1.StatusRequest{})
		_ = conn.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s status: %v\n", target.name, err)
			os.Exit(1)
		}
		if response.GetService() != target.name || response.GetState() != "ready" {
			fmt.Fprintf(os.Stderr, "%s unexpected status: %v\n", target.name, response)
			os.Exit(1)
		}
		fmt.Printf("%s: %s (%s)\n", response.GetService(), response.GetState(), response.GetVersion())
	}
}
