#!/usr/bin/env bash
set -euo pipefail

PROTO_DIR="$(cd "$(dirname "$0")" && pwd)"
export PATH="$(go env GOPATH)/bin:$PATH"

cd "$PROTO_DIR"
protoc \
  --proto_path=. \
  --go_out=. \
  --go_opt=module=noplanner/backend/proto \
  --go-grpc_out=. \
  --go-grpc_opt=module=noplanner/backend/proto \
  common/v1/error.proto \
  common/v1/language.proto \
  common/v1/progress.proto \
  common/v1/status.proto \
  gateway/v1/gateway.proto \
  user/v1/user.proto \
  project/v1/project.proto \
  document/v1/document.proto \
  intelligence/v1/intelligence.proto \
  research/v1/research.proto \
  evaluation/v1/evaluation.proto \
  generation/v1/generation.proto
