# Bruno

Open `collections/Agentinder` in Bruno and select `Local` or `Production`.
The collection checks the BFF, the backend's protected web route, and public
MCP initialization.

The backend token in both environments is only a placeholder. Store a real
value in Bruno's local secret storage when testing the protected backend
request; never commit it. Browser requests use the BFF and do not send this
token.
