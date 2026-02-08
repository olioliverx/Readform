#!/usr/bin/env sh
set -eu

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
REPO_ROOT="$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# Rebaseline dependency manifests from committed HEAD for deterministic regeneration.
git restore --source=HEAD -- go.mod go.sum

"$SCRIPT_DIR/sync-modules.sh"
docker build --platform linux/amd64 --no-cache --progress=plain -t readform:weekly-test .
