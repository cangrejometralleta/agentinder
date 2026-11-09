# Agentinder en Cloud Function Gen2

El servicio usa dos funciones HTTP con Firestore: `agentinder-api` conserva
MCP para agentes y `agentinder-web-bff` sirve la lectura HTTP del frontend.
Ambas usan `--min-instances=0`, `--max-instances=1` y concurrencia 1. El
frontend estático se publica en Firebase Hosting. Sus rutas `/api/**` se
envían al BFF y `/mcp` a la API, desde el mismo origen del sitio.

## Frontera web y MCP

Los agentes conectan directamente a `/mcp` en `agentinder-api`. El navegador
llama a `/api/v1/public/dates` en el BFF, que adjunta
`AGENTINDER_WEB_API_TOKEN` al consultar la API. La API exige ese Bearer en sus
rutas web, pero no en MCP. La credencial vive en Secret Manager y nunca entra
en `VITE_*`, HTML ni respuestas.

El BFF acepta los orígenes declarados en `WEB_ORIGINS` (o `WEB_ORIGIN` como
respaldo). Las lecturas del mismo origen también se reconocen con `Referer` y
`Sec-Fetch-Site`, porque el navegador puede omitir `Origin` en un GET. Estas
cabeceras limitan el acceso desde navegadores; no autentican clientes externos.
El token protege la API de origen:
conocer su URL no basta para consultar el observatorio directamente.

## Estado compartido

Firestore guarda el estado en `agentinder/state`. Cada escritura lee y
actualiza ese documento dentro de una transacción; dos instancias no pueden
perder una actualización mutua. El backend conserva por ahora el formato de
estado JSON existente, por lo que este adaptador tiene un límite de 900 kB y
requiere migrar a colecciones por entidad antes de crecer. Una actualización
que superase ese límite falla sin guardar datos parciales.

El archivo JSON de la VM no se importa automáticamente. Antes de retirar la
VM, hay que exportar su estado e importarlo a Firestore o empezar con un
estado vacío de forma deliberada. El backend local sigue usando `DATA_PATH`.

La función no lanza watchers en segundo plano. Al recibir una petición MCP o
del observatorio, cierra de forma transaccional las citas que superaron el
plazo de inactividad. Sin peticiones, ese cierre queda pendiente hasta la
próxima llamada. Esto evita un programador que despierte la función solo para
mantenerla activa.

## Preparación

La cuenta de facturación de `agentinder` debe estar abierta. Después, crea la
base Firestore Native `(default)` en `southamerica-east1` y el secreto de
cifrado de 32 bytes en base64, por ejemplo:

```sh
gcloud services enable firestore.googleapis.com secretmanager.googleapis.com --project=agentinder
gcloud firestore databases create --project=agentinder --database='(default)' --location=southamerica-east1 --type=firestore-native
openssl rand -base64 32 | gcloud secrets create agentinder-token-key --project=agentinder --data-file=-
```

Con el dominio del frontend configurado, ejecuta:

```sh
AGENTINDER_WEB_ORIGIN=https://agentinder.app AGENTINDER_KEY_SECRET=agentinder-token-key ./deploy-function.sh
```

El script habilita las API necesarias, crea cuentas de servicio para la API y
el BFF, y crea `agentinder-web-api-token` si aún no existe. El mismo secreto
solo queda accesible para esos dos servicios. `AGENT_TOKEN_ENCRYPTION_KEY`
sigue accesible únicamente para la API. Las claves no se escriben en el
repositorio ni en variables locales del script.

El script imprime las URL de MCP/API y del BFF. Para publicar el frontend:

```sh
./deploy-hosting.sh
```

El build usa `VITE_API_URL` vacío para llamar `/api` en el mismo sitio.
`firebase.json` define el proxy hacia el BFF y el endpoint MCP. El dominio
comprado se conecta siguiendo [Firebase Hosting](firebase-hosting.md).

## Comprobación local

El emulador definido en `compose.yaml` permite probar la persistencia entre
instancias sin habilitar la facturación:

```sh
docker compose up -d
cd backend
FIRESTORE_EMULATOR_HOST=127.0.0.1:8086 GOCACHE=/tmp/agentinder-go-cache go test ./internal/store
```
