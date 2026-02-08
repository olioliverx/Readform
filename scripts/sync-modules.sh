#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

docker run --rm \
  --platform linux/amd64 \
  --user "$(id -u):$(id -g)" \
  -e GOPROXY="https://proxy.golang.org,direct" \
  -e GOSUMDB="sum.golang.org" \
  -e GOMODCACHE="/tmp/gomodcache" \
  -e GOCACHE="/tmp/gocache" \
  -v "$(pwd)":/src \
  -w /src \
  golang:1.21-bullseye \
  sh -c 'mkdir -p "$GOMODCACHE" "$GOCACHE" && command -v go && go version && go mod tidy && go mod download && go mod verify'
