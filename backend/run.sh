#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

check_environment() {
  if [[ -z "${APP_ENV:-}" && ! -f .env ]]; then
    printf '%s\n' 'Falta la configuración. Copia .env.example a .env y vuelve a ejecutar.' >&2
    exit 1
  fi
}

check_environment
exec go run ./cmd/api
