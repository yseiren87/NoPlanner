module noplanner/backend-msa/gateway

go 1.26.0

require (
	github.com/alicebob/miniredis/v2 v2.37.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/redis/go-redis/v9 v9.17.2
	golang.org/x/net v0.48.0
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.11
	noplanner/proto v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)

replace noplanner/proto => ../../proto/dist/go
