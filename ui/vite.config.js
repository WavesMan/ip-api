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
            const target = `http://localhost:8080${req.url}`
            const r = await fetch(target)
            const text = await r.text()
            res.statusCode = r.status
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
