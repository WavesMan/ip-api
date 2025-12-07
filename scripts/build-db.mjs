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

  const dictPath = path.resolve(root, 'edge-functions', 'dict.js')
  if (fs.existsSync(dictPath)) {
    const mod = await import(pathToFileURL(dictPath).href)
    if (mod && mod.DICT) {
      await writeBin(mod.DICT, path.join(outDir, 'dict.bin'))
      process.stdout.write('dict.bin generated\n')
    } else {
      process.stdout.write('dict.js missing DICT export\n')
    }
  } else {
    process.stdout.write('dict.js not found, skip\n')
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
  process.stdout.write(`chunks generated: ${count}\n`)
}

main().catch((e) => {
  process.stderr.write(String(e) + '\n')
  process.exitCode = 1
})

