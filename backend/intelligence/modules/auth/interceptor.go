package auth

import (
 "context"
 "crypto/subtle"

 "google.golang.org/grpc"
 "google.golang.org/grpc/codes"
 "google.golang.org/grpc/metadata"
 "google.golang.org/grpc/status"
)

func UnaryServer(expected string, publicMethods ...string) grpc.UnaryServerInterceptor {
 public:=map[string]bool{};for _,method:=range publicMethods{public[method]=true}
 return func(ctx context.Context,req any,info *grpc.UnaryServerInfo,handler grpc.UnaryHandler)(any,error){
  if public[info.FullMethod]{return handler(ctx,req)}
  values:=metadata.ValueFromIncomingContext(ctx,"x-internal-service-token")
  if expected==""||len(values)!=1||subtle.ConstantTimeCompare([]byte(values[0]),[]byte(expected))!=1{return nil,status.Error(codes.Unauthenticated,"authenticated service call required")}
  return handler(ctx,req)
 }
}
func UnaryClient(token string) grpc.UnaryClientInterceptor {
 return func(ctx context.Context,method string,req,reply any,cc *grpc.ClientConn,invoker grpc.UnaryInvoker,opts ...grpc.CallOption)error{ctx=metadata.AppendToOutgoingContext(ctx,"x-internal-service-token",token);return invoker(ctx,method,req,reply,cc,opts...)}
}

