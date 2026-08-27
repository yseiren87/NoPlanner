package clients

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	documentv1 "noplanner/backend/proto/dist/golang/document/v1"
	evaluationv1 "noplanner/backend/proto/dist/golang/evaluation/v1"
	generationv1 "noplanner/backend/proto/dist/golang/generation/v1"
	projectv1 "noplanner/backend/proto/dist/golang/project/v1"
	researchv1 "noplanner/backend/proto/dist/golang/research/v1"
	userv1 "noplanner/backend/proto/dist/golang/user/v1"
)

type Addresses struct{ User, Project, Document, Research, Evaluation, Generation string }
type Clients struct {
	User        userv1.UserServiceClient
	Project     projectv1.ProjectServiceClient
	Document    documentv1.DocumentServiceClient
	Research    researchv1.ResearchServiceClient
	Evaluation  evaluationv1.EvaluationServiceClient
	Generation  generationv1.GenerationServiceClient
	connections []*grpc.ClientConn
}

func New(a Addresses, internalToken string) (*Clients, error) {
	values := []string{a.User, a.Project, a.Document, a.Research, a.Evaluation, a.Generation}
	for _, value := range values {
		if value == "" {
			return nil, fmt.Errorf("all downstream gRPC addresses are required")
		}
	}
	c := &Clients{}
	interceptor := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		ctx = metadata.AppendToOutgoingContext(ctx, "x-internal-service-token", internalToken)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	dial := func(address string) (*grpc.ClientConn, error) {
		conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(interceptor))
		if err == nil {
			c.connections = append(c.connections, conn)
		}
		return conn, err
	}
	u, e := dial(a.User)
	if e != nil {
		return nil, e
	}
	p, e := dial(a.Project)
	if e != nil {
		return nil, e
	}
	d, e := dial(a.Document)
	if e != nil {
		return nil, e
	}
	r, e := dial(a.Research)
	if e != nil {
		return nil, e
	}
	ev, e := dial(a.Evaluation)
	if e != nil {
		return nil, e
	}
	g, e := dial(a.Generation)
	if e != nil {
		return nil, e
	}
	c.User = userv1.NewUserServiceClient(u)
	c.Project = projectv1.NewProjectServiceClient(p)
	c.Document = documentv1.NewDocumentServiceClient(d)
	c.Research = researchv1.NewResearchServiceClient(r)
	c.Evaluation = evaluationv1.NewEvaluationServiceClient(ev)
	c.Generation = generationv1.NewGenerationServiceClient(g)
	return c, nil
}
func (c *Clients) Close() {
	for _, conn := range c.connections {
		_ = conn.Close()
	}
}
