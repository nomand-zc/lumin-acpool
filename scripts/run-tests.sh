#!/usr/bin/env bash
set -euo pipefail

echo "Running go test with race detection..."
go test -race -count=1 ./...
