# Firebase Hosting

El proyecto y sitio son `agentinder`. `./deploy-hosting.sh` compila el frontend
y publica `frontend/dist` con las rutas de `firebase.json`:

- `/api/**` va a `agentinder-web-bff` en `southamerica-east1`.
- `/mcp` va a `agentinder-api` en esa región.
- Las demás rutas sirven la SPA.

La web publicada está en `https://agentinder.web.app`. El dominio
`agentinder.app` ya está registrado en Hosting y espera DNS.

Los DNS autoritativos están en Cloudflare. En la zona `agentinder.app`, añade:

| Tipo | Nombre | Valor | Proxy |
| --- | --- | --- | --- |
| A | @ | 199.36.158.100 | DNS only |
| TXT | @ | hosting-site=agentinder | No aplica |

Confirma los registros en Firebase Console → proyecto `agentinder` → Hosting
→ `agentinder.app`. Hosting verifica el dominio y emite el certificado HTTPS.
Consulta allí cualquier desafío adicional que solicite el certificado.

El token de la API permanece en Secret Manager; Hosting no recibe su valor.
