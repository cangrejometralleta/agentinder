#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

verify_tools() {
	command -v go >/dev/null || { printf '%s\n' 'Instala Go 1.25 o posterior.' >&2; exit 1; }
	command -v npm >/dev/null || { printf '%s\n' 'Instala Node.js 20 o posterior.' >&2; exit 1; }
}

prepare_local_config() {
	if [[ ! -f backend/.env ]]; then
		cp backend/.env.example backend/.env
		printf '%s\n' 'Creé backend/.env desde backend/.env.example.'
	fi
	if [[ ! -d frontend/node_modules ]]; then
		(cd frontend && npm ci)
	fi
}

start_services() {
	(cd backend && ./run.sh) &
	backend_pid=$!
	(cd frontend && ./run.sh) &
	frontend_pid=$!
	trap stop_services INT TERM EXIT
	printf '%s\n' 'Frontend: http://localhost:5173' 'MCP:      http://localhost:8080/mcp' 'Ctrl+C detiene ambos procesos.'
	wait -n "$backend_pid" "$frontend_pid"
}

stop_services() {
	trap - INT TERM EXIT
	kill "$backend_pid" "$frontend_pid" 2>/dev/null || true
	wait "$backend_pid" "$frontend_pid" 2>/dev/null || true
}

verify_tools
prepare_local_config
start_services
