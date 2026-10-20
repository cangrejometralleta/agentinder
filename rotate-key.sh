#!/usr/bin/env bash
# Rota AGENT_TOKEN_ENCRYPTION_KEY en la VM sin perder las respuestas vigentes.
#   ./rotate-key.sh start    genera una clave nueva en la VM y deja la actual como anterior
#   ./rotate-key.sh status   muestra si hay una rotación en curso
#   ./rotate-key.sh finish   retira la clave anterior; exige que pase la ventana (o --force)
set -euo pipefail
cd -- "$(dirname -- "$0")"

usage() { printf '%s\n' 'Uso: ./rotate-key.sh {start|status|finish [--force]}' >&2; exit 2; }

load_config() {
	if [[ -f config/deploy.env ]]; then
		set -a
		. config/deploy.env
		set +a
	fi
	: "${DEPLOY_HOST:?Copia config/deploy.env.example a config/deploy.env y define DEPLOY_HOST}"
	: "${DEPLOY_USER:?Define DEPLOY_USER en config/deploy.env}"
	DEPLOY_ENV_FILE="${DEPLOY_ENV_FILE:-/etc/agentinder/agentinder.env}"
	KEY_ROTATION_WINDOW_HOURS="${KEY_ROTATION_WINDOW_HOURS:-24}"
	DEPLOY_HEALTH_URL="${DEPLOY_HEALTH_URL:-http://127.0.0.1:8080}"
}

verify_config() {
	[[ "$DEPLOY_HOST" =~ ^[A-Za-z0-9._-]+$ ]] || { printf '%s\n' 'DEPLOY_HOST contiene caracteres no admitidos.' >&2; exit 1; }
	[[ "$DEPLOY_USER" =~ ^[A-Za-z0-9._-]+$ ]] || { printf '%s\n' 'DEPLOY_USER contiene caracteres no admitidos.' >&2; exit 1; }
	[[ "$DEPLOY_ENV_FILE" =~ ^/[A-Za-z0-9._/-]+$ ]] || { printf '%s\n' 'DEPLOY_ENV_FILE debe ser una ruta absoluta sin espacios.' >&2; exit 1; }
	[[ "$KEY_ROTATION_WINDOW_HOURS" =~ ^[0-9]+$ ]] || { printf '%s\n' 'KEY_ROTATION_WINDOW_HOURS debe ser un número entero de horas.' >&2; exit 1; }
	[[ "$DEPLOY_HEALTH_URL" =~ ^http://127\.0\.0\.1:[0-9]+$ ]] || { printf '%s\n' 'DEPLOY_HEALTH_URL debe apuntar a 127.0.0.1 dentro de la VM.' >&2; exit 1; }
	command -v ssh >/dev/null || { printf '%s\n' 'Falta la herramienta requerida: ssh' >&2; exit 1; }
}

action="${1:-}"
force="${2:-}"
case "$action" in
start | status) [[ -z "$force" ]] || usage ;;
finish) [[ -z "$force" || "$force" == "--force" ]] || usage ;;
*) usage ;;
esac

load_config
verify_config
ssh "$DEPLOY_USER@$DEPLOY_HOST" bash -s -- "$action" "$DEPLOY_ENV_FILE" "$KEY_ROTATION_WINDOW_HOURS" "$DEPLOY_HEALTH_URL" "$force" < rotate-key.remote.sh
