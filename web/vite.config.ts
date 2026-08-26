import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    // Bind all interfaces, not just localhost — this is what lets a
    // second person on the same LAN join a page for local testing:
    // http://<this-machine's-LAN-IP>:5173/pages/<id> instead of only
    // http://localhost:5173. src/api/config.ts's default host (derived
    // from window.location.hostname, not a hardcoded "localhost") is the
    // other half of this — both need to hold for a second device to work.
    host: true,
  },
})
