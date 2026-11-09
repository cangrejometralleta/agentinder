#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

build_frontend() {
	(cd frontend && VITE_API_URL= npm run build)
}

publish_hosting() {
	firebase deploy --only hosting --project agentinder --non-interactive
}

build_frontend
publish_hosting
