import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [
    vue(),
    tailwindcss(),
    {
      name: 'ip-api-dev-proxy',
      configureServer(server) {
        server.middlewares.use('/api/ip', async (req, res) => {
          try {
            const u = new URL(req.url, 'http://localhost')
            const mod = await import('./edge-functions/ip-lookup.js')
            const headers = new Headers()
            for (const [k, v] of Object.entries(req.headers)) {
              if (typeof v === 'string') headers.set(k, v)
            }
            const r = await mod.default(new Request(u.toString(), { headers }))
            const text = await r.text()
            res.statusCode = 200
            res.setHeader('content-type', 'application/json; charset=utf-8')
            res.setHeader('cache-control', 'no-store')
            res.end(text)
          } catch (e) {
            res.statusCode = 500
            res.end(JSON.stringify({ error: 'dev proxy error' }))
          }
        })
        server.middlewares.use('/api/stats', async (req, res) => {
          try {
            const u = new URL(req.url, 'http://localhost')
            const mod = await import('./edge-functions/stats.js')
            const r = await mod.default(new Request(u.toString()))
            const text = await r.text()
            res.statusCode = 200
            res.setHeader('content-type', 'application/json; charset=utf-8')
            res.setHeader('cache-control', 'no-store')
            res.end(text)
          } catch (e) {
            res.statusCode = 500
            res.end(JSON.stringify({ error: 'dev proxy error' }))
          }
        })
      },
    },
  ],
})
