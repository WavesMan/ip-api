import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)
const root = path.resolve(__dirname, '..')
const outDir = path.resolve(root, 'public', 'db')
const outChunks = path.join(outDir, 'chunks')

function ensureDir(p) {
  if (!fs.existsSync(p)) fs.mkdirSync(p, { recursive: true })
}

function ipToInt(ip) {
  const p = ip.split('.')
  return ((Number(p[0]) << 24) >>> 0) + (Number(p[1]) << 16) + (Number(p[2]) << 8) + Number(p[3])
}

function writeVar(arr, x) {
  let v = x >>> 0
  while (true) {
    const b = v & 0x7f
    v >>>= 7
    arr.push(v ? (b | 0x80) : b)
    if (!v) break
  }
}

async function fetchSource() {
  const urls = [
    'https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ipv4_source.txt',
    'https://cdn.jsdelivr.net/gh/lionsoul2014/ip2region@master/data/ipv4_source.txt',
  ]
  for (const u of urls) {
    try {
      const res = await fetch(u)
      if (res.ok) return await res.text()
    } catch {}
  }
  throw new Error('无法获取上游数据集')
}

async function main() {
  ensureDir(outDir)
  ensureDir(outChunks)

  const text = await fetchSource()
  const lines = text.split(/\r?\n/).filter(Boolean)

  const strIndex = new Map()
  const strings = []
  function intern(s) {
    if (!strIndex.has(s)) {
      strIndex.set(s, strings.length)
      strings.push(s)
    }
    return strIndex.get(s)
  }
  const tripleIndex = new Map()
  const triples = []
  function internTriple(c, p, ci) {
    const key = `${c}|${p}|${ci}`
    if (!tripleIndex.has(key)) {
      const a = intern(c)
      const b = intern(p)
      const d = intern(ci)
      tripleIndex.set(key, triples.length)
      triples.push([a, b, d])
    }
    return tripleIndex.get(key)
  }

  const buckets = Array.from({ length: 256 }, () => [])

  for (const line of lines) {
    const parts = line.split('|')
    if (parts.length < 6) continue
    const start = ipToInt(parts[0])
    const end = ipToInt(parts[1])
    const country = parts[2] || ''
    const province = parts[3] || ''
    const city = parts[4] || ''
    const tri = internTriple(country, province, city)
    const a = (start >>> 24) & 0xff
    buckets[a].push([start, end, tri])
  }

  const dict = []
  dict.push(0x49, 0x50, 0x44, 0x43)
  dict.push(1)
  const dvh = Buffer.alloc(8)
  new DataView(dvh.buffer).setUint32(0, strings.length, true)
  new DataView(dvh.buffer).setUint32(4, triples.length, true)
  dict.push(...dvh)
  for (const s of strings) {
    const b = Buffer.from(s)
    const h = Buffer.alloc(2)
    new DataView(h.buffer).setUint16(0, b.length, true)
    dict.push(...h)
    dict.push(...b)
  }
  for (const [a, b, c] of triples) {
    const h = Buffer.alloc(6)
    const dv = new DataView(h.buffer)
    dv.setUint16(0, a, true)
    dv.setUint16(2, b, true)
    dv.setUint16(4, c, true)
    dict.push(...h)
  }
  fs.writeFileSync(path.join(outDir, 'dict.bin'), Buffer.from(dict))

  let chunkCount = 0
  for (let a = 0; a < 256; a++) {
    const recs = buckets[a]
    if (recs.length === 0) continue
    recs.sort((x, y) => x[0] - y[0])
    const bytes = []
    bytes.push(0x49, 0x50, 0x43, 0x48)
    bytes.push(1)
    const h = Buffer.alloc(4)
    new DataView(h.buffer).setUint32(0, recs.length, true)
    bytes.push(...h)
    let prev = 0
    for (const [start, end, tri] of recs) {
      const delta = (start - prev) >>> 0
      const len = (end - start) >>> 0
      writeVar(bytes, delta)
      writeVar(bytes, len)
      const t = Buffer.alloc(2)
      new DataView(t.buffer).setUint16(0, tri, true)
      bytes.push(...t)
      prev = start >>> 0
    }
    fs.writeFileSync(path.join(outChunks, `a${a}.bin`), Buffer.from(bytes))
    chunkCount++
  }

  process.stdout.write(`source lines: ${lines.length}\n`)
  process.stdout.write(`strings: ${strings.length} triples: ${triples.length}\n`)
  process.stdout.write(`chunks generated: ${chunkCount}\n`)
}

main().catch((e) => {
  process.stderr.write(String(e) + '\n')
  process.exitCode = 1
})

