import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const root = path.resolve(__dirname, '..')
const outDir = path.resolve(root, 'public', 'db')
const outChunks = path.join(outDir, 'chunks')

function ensureDir(p) {
  if (!fs.existsSync(p)) fs.mkdirSync(p, { recursive: true })
}

async function writeBin(buf, to) {
  ensureDir(path.dirname(to))
  const b = Buffer.from(buf)
  fs.writeFileSync(to, b)
}

async function main() {
  ensureDir(outDir)
  ensureDir(outChunks)

  let dictDone = false
  let chunksDone = false
  const dictPath = path.resolve(root, 'edge-functions', 'dict.js')
  if (fs.existsSync(dictPath)) {
    const mod = await import(pathToFileURL(dictPath).href)
    if (mod && mod.DICT) {
      await writeBin(mod.DICT, path.join(outDir, 'dict.bin'))
      dictDone = true
    }
  }
  const chunksDir = path.resolve(root, 'edge-functions', 'chunks')
  let count = 0
  for (let a = 0; a < 256; a += 1) {
    const f = path.join(chunksDir, `a${a}.js`)
    if (!fs.existsSync(f)) continue
    const mod = await import(pathToFileURL(f).href)
    if (mod && mod.CH) {
      await writeBin(mod.CH, path.join(outChunks, `a${a}.bin`))
      count += 1
    }
  }
  if (count > 0) chunksDone = true

  if (!dictDone || !chunksDone) {
    const dataPath = path.resolve(root, 'edge-functions', 'data.js')
    if (fs.existsSync(dataPath)) {
      const mod = await import(pathToFileURL(dataPath).href)
      if (mod && mod.DB) {
        const DB = Buffer.from(mod.DB)
        const findAll = (magic) => {
          const idxs = []
          const m = Buffer.from(magic)
          let i = 0
          while (true) {
            const p = DB.indexOf(m, i)
            if (p === -1) break
            idxs.push(p)
            i = p + 1
          }
          return idxs
        }
        const ipdc = findAll('IPDC')
        const ipch = findAll('IPCH')
        if (!dictDone && ipdc.length > 0) {
          const start = ipdc[0]
          const end = ipch.length > 0 ? ipch[0] : DB.length
          const slice = DB.subarray(start, end)
          await writeBin(slice, path.join(outDir, 'dict.bin'))
          dictDone = true
        }
        if (!chunksDone && ipch.length > 0) {
          for (let a = 0; a < ipch.length; a += 1) {
            const start = ipch[a]
            const end = a + 1 < ipch.length ? ipch[a + 1] : DB.length
            const slice = DB.subarray(start, end)
            await writeBin(slice, path.join(outChunks, `a${a}.bin`))
            count += 1
          }
          chunksDone = true
        }
      }
    }
  }

  process.stdout.write(`dict: ${dictDone ? 'ok' : 'skip'}\n`)
  process.stdout.write(`chunks generated: ${count}\n`)
}

main().catch((e) => {
  process.stderr.write(String(e) + '\n')
  process.exitCode = 1
})
