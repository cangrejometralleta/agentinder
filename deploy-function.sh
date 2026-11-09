#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"

: "${AGENTINDER_WEB_ORIGIN:?Define AGENTINDER_WEB_ORIGIN como origen HTTPS del frontend}"
: "${AGENTINDER_KEY_SECRET:?Define AGENTINDER_KEY_SECRET como nombre del secreto en Secret Manager}"

PROJECT=agentinder
REGION=southamerica-east1
FUNCTION=agentinder-api
BFF_FUNCTION=agentinder-web-bff
WEB_TOKEN_SECRET=${AGENTINDER_WEB_API_TOKEN_SECRET:-agentinder-web-api-token}
SERVICE_ACCOUNT="agentinder-api@$PROJECT.iam.gserviceaccount.com"
BFF_SERVICE_ACCOUNT="agentinder-web-bff@$PROJECT.iam.gserviceaccount.com"
WEB_ORIGINS=${AGENTINDER_WEB_ORIGINS:-$AGENTINDER_WEB_ORIGIN,https://$PROJECT.web.app,https://$PROJECT.firebaseapp.com}

[[ "$AGENTINDER_WEB_ORIGIN" =~ ^https://[A-Za-z0-9.-]+(:[0-9]+)?$ ]] || {
	printf '%s\n' 'AGENTINDER_WEB_ORIGIN debe ser un origen HTTPS.' >&2
	exit 1
}

gcloud services enable cloudfunctions.googleapis.com run.googleapis.com cloudbuild.googleapis.com \
	artifactregistry.googleapis.com firestore.googleapis.com secretmanager.googleapis.com --project="$PROJECT"

if ! gcloud iam service-accounts describe "$SERVICE_ACCOUNT" --project="$PROJECT" >/dev/null 2>&1; then
	gcloud iam service-accounts create agentinder-api --project="$PROJECT" --display-name='Agentinder API'
fi
if ! gcloud iam service-accounts describe "$BFF_SERVICE_ACCOUNT" --project="$PROJECT" >/dev/null 2>&1; then
	gcloud iam service-accounts create agentinder-web-bff --project="$PROJECT" --display-name='Agentinder Web BFF'
fi

if ! gcloud secrets describe "$WEB_TOKEN_SECRET" --project="$PROJECT" >/dev/null 2>&1; then
	openssl rand -hex 32 | tr -d '\n' | gcloud secrets create "$WEB_TOKEN_SECRET" --project="$PROJECT" --data-file=- --replication-policy=automatic
fi

gcloud projects add-iam-policy-binding "$PROJECT" --member="serviceAccount:$SERVICE_ACCOUNT" \
	--role=roles/datastore.user --quiet >/dev/null
gcloud secrets add-iam-policy-binding "$AGENTINDER_KEY_SECRET" --project="$PROJECT" \
	--member="serviceAccount:$SERVICE_ACCOUNT" --role=roles/secretmanager.secretAccessor --quiet >/dev/null
gcloud secrets add-iam-policy-binding "$WEB_TOKEN_SECRET" --project="$PROJECT" \
	--member="serviceAccount:$SERVICE_ACCOUNT" --role=roles/secretmanager.secretAccessor --quiet >/dev/null
gcloud secrets add-iam-policy-binding "$WEB_TOKEN_SECRET" --project="$PROJECT" \
	--member="serviceAccount:$BFF_SERVICE_ACCOUNT" --role=roles/secretmanager.secretAccessor --quiet >/dev/null

gcloud firestore databases describe --project="$PROJECT" --database='(default)' >/dev/null
gcloud secrets describe "$AGENTINDER_KEY_SECRET" --project="$PROJECT" >/dev/null
gcloud secrets describe "$WEB_TOKEN_SECRET" --project="$PROJECT" >/dev/null

gcloud functions deploy "$FUNCTION" --gen2 --trigger-http --allow-unauthenticated \
	--project="$PROJECT" --region="$REGION" --source=backend --entry-point=ServeAgentinder \
	--runtime=go126 --memory=512Mi --timeout=120s --min-instances=0 --max-instances=1 \
	--concurrency=1 --service-account="$SERVICE_ACCOUNT" \
	--set-build-env-vars=GOFLAGS=-buildvcs=false \
	--set-env-vars="GOOGLE_CLOUD_PROJECT=$PROJECT,WEB_ORIGIN=$AGENTINDER_WEB_ORIGIN" \
	--set-secrets="AGENT_TOKEN_ENCRYPTION_KEY=$AGENTINDER_KEY_SECRET:latest,AGENTINDER_WEB_API_TOKEN=$WEB_TOKEN_SECRET:latest"

API_URL=$(gcloud functions describe "$FUNCTION" --gen2 --region="$REGION" --project="$PROJECT" --format='value(serviceConfig.uri)')
[[ "$API_URL" =~ ^https:// ]] || {
	printf '%s\n' 'No se pudo resolver la URL interna de la API.' >&2
	exit 1
}

gcloud functions deploy "$BFF_FUNCTION" --gen2 --trigger-http --allow-unauthenticated \
	--project="$PROJECT" --region="$REGION" --source=backend --entry-point=ServeAgentinderBFF \
	--runtime=go126 --memory=256Mi --timeout=30s --min-instances=0 --max-instances=1 \
	--concurrency=1 --service-account="$BFF_SERVICE_ACCOUNT" \
	--set-build-env-vars=GOFLAGS=-buildvcs=false \
	--set-env-vars="^|^AGENTINDER_API_URL=$API_URL|WEB_ORIGIN=$AGENTINDER_WEB_ORIGIN|WEB_ORIGINS=$WEB_ORIGINS" \
	--set-secrets="AGENTINDER_WEB_API_TOKEN=$WEB_TOKEN_SECRET:latest"

BFF_URL=$(gcloud functions describe "$BFF_FUNCTION" --gen2 --region="$REGION" --project="$PROJECT" --format='value(url)')
[[ "$BFF_URL" =~ ^https:// ]] || {
	printf '%s\n' 'No se pudo resolver la URL del BFF.' >&2
	exit 1
}
printf 'API MCP: %s\nBFF web: %s\n' \
	"$(gcloud functions describe "$FUNCTION" --gen2 --region="$REGION" --project="$PROJECT" --format='value(url)')" \
	"$BFF_URL"
