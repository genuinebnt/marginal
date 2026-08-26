// Service base URLs — the browser calls these directly, not through the
// `web` dev-server container's own network (docker-compose.yml's `web`
// service only serves the SPA's static assets/HMR; every fetch/WebSocket
// below runs in the *browser*, which reaches services via the same
// host-published ports `docker compose up` exposes). Overridable via Vite
// env vars for anything other than the default local docker-compose
// setup — see .env.example.
//
// Defaulting to window.location.hostname (not the literal "localhost")
// is what makes local multi-device testing work at all: a second person
// joining from another machine on the same LAN reaches the Vite dev
// server at the host's LAN IP (e.g. http://192.168.1.23:5173/pages/<id>,
// with vite.config.ts's server.host binding all interfaces for exactly
// this), and their browser's "localhost" would resolve to their own
// machine, not the host's — every API/WS call would silently 404/refuse.
// Reusing whatever host the page itself was loaded from sidesteps that
// without needing per-viewer configuration.
const defaultHost = typeof window !== "undefined" ? window.location.hostname : "localhost";

export const GATEWAY_URL = import.meta.env.VITE_GATEWAY_URL ?? `http://${defaultHost}:8000`;
export const COLLAB_URL = import.meta.env.VITE_COLLAB_URL ?? `http://${defaultHost}:8002`;
export const NOTIFICATIONS_URL = import.meta.env.VITE_NOTIFICATIONS_URL ?? `http://${defaultHost}:8007`;
