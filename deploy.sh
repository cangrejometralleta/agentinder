#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

load_deploy_config() {
	if [[ -f config/deploy.env ]]; then
		set -a
		. config/deploy.env
		set +a
	fi
	: "${DEPLOY_HOST:?Copia config/deploy.env.example a config/deploy.env y define DEPLOY_HOST}"
	: "${DEPLOY_USER:?Define DEPLOY_USER en config/deploy.env}"
	: "${DEPLOY_ROOT:?Define DEPLOY_ROOT en config/deploy.env}"
	: "${PUBLIC_ORIGIN:?Define PUBLIC_ORIGIN en config/deploy.env}"
}

verify_deploy_config() {
	[[ "$DEPLOY_HOST" =~ ^[A-Za-z0-9._-]+$ ]] || { printf '%s\n' 'DEPLOY_HOST contiene caracteres no admitidos.' >&2; exit 1; }
	[[ "$DEPLOY_USER" =~ ^[A-Za-z0-9._-]+$ ]] || { printf '%s\n' 'DEPLOY_USER contiene caracteres no admitidos.' >&2; exit 1; }
	[[ "$DEPLOY_ROOT" =~ ^/[A-Za-z0-9._/-]+$ ]] || { printf '%s\n' 'DEPLOY_ROOT debe ser una ruta absoluta sin espacios.' >&2; exit 1; }
	[[ "$PUBLIC_ORIGIN" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]] || { printf '%s\n' 'PUBLIC_ORIGIN debe ser un origen HTTPS.' >&2; exit 1; }
	for tool in git go npm ssh scp tar; do
		command -v "$tool" >/dev/null || { printf 'Falta la herramienta requerida: %s\n' "$tool" >&2; exit 1; }
	done
}

build_release() {
	release="$(date -u +%Y%m%dT%H%M%SZ)-$(git rev-parse --short HEAD)"
	stage="$(mktemp -d)"
	archive="$(mktemp)"
	trap 'rm -rf "$stage"; rm -f "$archive"' EXIT
	mkdir -p "$stage/backend/bin" "$stage/frontend/dist"
	(cd backend && go build -trimpath -o "$stage/backend/bin/agentinder" ./cmd/api)
	(cd frontend && npm ci && VITE_API_URL= npm run build)
	cp -a frontend/dist/. "$stage/frontend/dist/"
	tar -czf "$archive" -C "$stage" .
}

publish_release() {
	target="$DEPLOY_USER@$DEPLOY_HOST"
	remote_archive="/tmp/agentinder-$release.tar.gz"
	scp "$archive" "$target:$remote_archive"
	ssh "$target" bash -s -- "$DEPLOY_ROOT" "$release" "$remote_archive" "$PUBLIC_ORIGIN" <<'REMOTE'
set -euo pipefail
root="$1"
release="$root/releases/$2"
archive="$3"
origin="$4"
mkdir -p "$release"
tar -xzf "$archive" -C "$release"
rm -f "$archive"
previous="$(readlink -f "$root/current" 2>/dev/null || true)"
ln -sfn "$release" "$root/current.next"
mv -Tf "$root/current.next" "$root/current"
if ! sudo systemctl restart agentinder \
	|| ! curl --fail --silent --show-error http://127.0.0.1:8080/healthz >/dev/null \
	|| ! curl --fail --silent --show-error http://127.0.0.1:8080/readyz >/dev/null; then
	if [[ -n "$previous" ]]; then
		ln -sfn "$previous" "$root/current.next"
		mv -Tf "$root/current.next" "$root/current"
		sudo systemctl restart agentinder || true
	fi
	printf '%s\n' 'El servicio no pasó healthz/readyz; se restauró el release anterior si existía.' >&2
	exit 1
fi
printf 'Release %s activo en %s\n' "$release" "$origin"
REMOTE
}

load_deploy_config
verify_deploy_config
build_release
publish_release
