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
        server.middlewares.use('/api', async (req, res) => {
          try {
            const target = `http://localhost:8080${req.url}`
            const headers = {
              'x-real-ip': req.socket?.remoteAddress || '',
              'x-forwarded-for': (req.headers['x-forwarded-for'] ? String(req.headers['x-forwarded-for']) : (req.socket?.remoteAddress || '')),
            }
            const r = await fetch(target, { headers })
            const text = await r.text()
            res.statusCode = r.status
            const ct = r.headers.get('content-type') || 'application/json; charset=utf-8'
            res.setHeader('content-type', ct)
            res.setHeader('cache-control', 'no-store')
            res.end(text)
          } catch (e) {
            res.statusCode = 500
            res.setHeader('content-type', 'application/json; charset=utf-8')
            res.end(JSON.stringify({ error: 'dev proxy error' }))
          }
        })
        server.middlewares.use('/config.js', async (req, res) => {
          try {
            const target = `http://localhost:8080/config.js`
            const r = await fetch(target)
            const text = await r.text()
            res.statusCode = r.status
            res.setHeader('content-type', r.headers.get('content-type') || 'application/javascript; charset=utf-8')
            res.setHeader('cache-control', 'no-store')
            res.end(text)
          } catch (e) {
            res.statusCode = 500
            res.setHeader('content-type', 'application/javascript; charset=utf-8')
            res.end("window.__API_BASE__='/api'")
          }
        })
      },
    },
  ],
})
