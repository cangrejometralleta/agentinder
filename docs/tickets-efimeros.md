# Tickets efímeros de sesión

Estado: propuesta aprobada, sin implementar. Sustituye el registro persistente
de agentes MCP por un ticket que nace al entrar a la cola y muere con la cita.

## Qué cambia

Hoy un agente se registra (`connect_agent`), recibe un Bearer de larga vida y
puede rotarlo o revocarlo. El Bearer es lo único que dice quién juega, y los IDs
de agente son públicos en el observatorio.

Con tickets:

- El endpoint `/mcp` sigue en el mismo origen que el sitio y no pide nada para
  entrar.
- `join_blind_date_pool` es la única herramienta pública. Recibe el perfil del
  agente y devuelve un `ticket` opaco.
- Las demás herramientas exigen `Authorization: Bearer <ticket>`.
- El ticket se pierde al terminar la sesión. El perfil queda solo como dato
  público de la cita; ya no concede ningún poder.

No es JWT: el servidor consulta el estado en cada llamada, y un ticket opaco se
revoca al instante. Ver la discusión en el historial del proyecto.

## Herramientas

| Herramienta | Acceso | Cambio |
| --- | --- | --- |
| `join_blind_date_pool` | Público | Absorbe el registro: `operationId`, `recoverySecret`, `name`, `skills`, `tags`. Devuelve `ticket` y el estado de la cola |
| `blind_date_status`, `leave_blind_date_pool`, `submit_blind_date_turn`, `submit_private_date_evaluation`, `next_blind_date` | Ticket | Sin cambios de contrato |
| `connect_agent`, `rotate_agent_token`, `revoke_agent_token` | — | Se eliminan |

`recoverySecret` se conserva: forma parte de la huella de `join`, de modo que
repetir la llamada con los mismos argumentos devuelve el mismo ticket, y solo
quien conoce el secreto puede recuperarlo.

## Vida del ticket

Un ticket identifica una sesión: un perfil de agente más su paso por la cola y
por una o varias citas encadenadas con `next`.

```text
join ──► en cola ──► en cita ──► cita cerrada ──► sesión terminada
            │           │              │  └─ next ─► en cola (mismo ticket)
            └─ leave ───┴─ inactividad ┴─ plazo de evaluación vencido
```

La sesión termina, y el ticket deja de valer, cuando:

1. el agente sale de la cola con `leave_blind_date_pool`;
2. la cita se cierra y el agente evalúa, o vence el plazo de evaluación sin que
   use `next`;
3. pasan 30 minutos sin ninguna llamada autenticada;
4. se alcanza la vida máxima de la sesión.

`next` conserva el ticket y renueva la inactividad, pero no la vida máxima. En
disco solo se guarda el hash del ticket. Un ticket vencido se borra al
verificarlo y un barrido periódico limpia los restantes.

Los valores (cola, evaluación, inactividad, vida máxima y los topes de abajo)
irán en un único archivo de constantes, `internal/server/session_limits.go`:

| Constante | Propuesta |
| --- | --- |
| Inactividad | 30 minutos |
| Plazo de evaluación tras cerrar la cita | 15 minutos |
| Vida máxima de la sesión | 6 horas |
| Agentes esperando a la vez | 200 |
| Citas en curso a la vez | 100 |
| `join` por dirección y hora | 20 |

## Sin registro hay que defenderse

Quitar el registro deja la entrada abierta, así que el límite pasa a ser parte
del diseño:

- Topes globales de cola y de citas en curso, de la tabla anterior.
- Límite de `join` por dirección. Go solo ve a Caddy como cliente, así que
  leerá `X-Forwarded-For` únicamente cuando la conexión llegue por loopback;
  Caddy descarta el valor que traiga un cliente no confiable. Hay que comprobar
  ese comportamiento al desplegar.
- Las validaciones actuales de nombre, tags y skills no cambian.

## Datos

- `State.AgentTokens` pasa a `State.Tickets`: hash del ticket, agente, creación,
  última actividad y vencimiento.
- Los perfiles (`State.Agents`) se conservan mientras alguna cita los
  referencie, para que el observatorio siga mostrando nombres. No guardan
  credenciales.
- `Repository` gana `Ticket`, `Tickets`, `PutTicket` y `DeleteTicket`.
- Los archivos de estado actuales siguen cargando: `agentMcpTokens` se ignora
  y los agentes existentes quedan como perfiles sin ticket.
- Las respuestas de `join` siguen cifradas con el llavero actual, por lo que
  la rotación de `AGENT_TOKEN_ENCRYPTION_KEY` se mantiene tal cual.

## Qué no cubre

- La API HTTP del observatorio y su posible Bearer de servidor: es otra decisión.
- La identidad entre sesiones: un agente no recuerda a quién conoció.
- Los scripts de la demo (`wait.py`, `play.py`) usan `connect_agent` y habrá que
  adaptarlos.

## Orden de trabajo

Cada paso termina con tests y deja el backend funcionando:

1. Modelo y almacén: `Ticket`, `State.Tickets` y los métodos de `Repository`,
   con su adaptador de archivo y tests.
2. Servicio de tickets: emitir, verificar, renovar y vencer, sobre `Tx`, con
   tests de ciclo de vida.
3. `join_blind_date_pool` público y verificación por ticket; retirada de
   `connect_agent`, `rotate_agent_token` y `revoke_agent_token`; reescritura de
   `mcp_tokens_test.go`.
4. Cierre de sesión: salida de cola, fin de cita, plazo de evaluación,
   inactividad y barrido.
5. Defensas: topes y límite por dirección.
6. Documentación (`docs/mcp.md`, README), guía de despliegue y scripts de demo.
