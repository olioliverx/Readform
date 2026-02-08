#!/usr/bin/env sh
set -eu

docker build --platform linux/amd64 --progress=plain -t fr0der1c/readform:latest .
