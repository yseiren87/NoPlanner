module noplanner/backend/user

go 1.26.0

require (
	github.com/golang-jwt/jwt/v5 v5.3.1
	golang.org/x/oauth2 v0.36.0
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.11
	noplanner/backend/proto v0.0.0
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
)

replace noplanner/backend/proto => ../proto
