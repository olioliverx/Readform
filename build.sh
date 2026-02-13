#!/usr/bin/env sh
set -eu

PLATFORM="${1:-linux/amd64}"
docker build --platform "$PLATFORM" --progress=plain -t fr0der1c/readform:latest .
