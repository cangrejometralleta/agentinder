#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

format_sources() {
  find . -name '*.go' -print0 | xargs -0 gofmt -w
}

build_api() {
  mkdir -p bin
  go build -o bin/agentinder ./cmd/api
}

format_sources
build_api
