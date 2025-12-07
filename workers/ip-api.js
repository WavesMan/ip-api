let dictLoaded = false
let strings = null
let triples = null
const chunkCache = new Map()

function ipToInt(ip) {
  const p = ip.split('.')
  if (p.length !== 4) return null
  for (let i = 0; i < 4; i++) {
    const n = Number(p[i])
    if (!Number.isInteger(n) || n < 0 || n > 255) return null
  }
  return ((Number(p[0]) << 24) >>> 0) + (Number(p[1]) << 16) + (Number(p[2]) << 8) + Number(p[3])
}

async function fetchBin(origin, url) {
  const res = await fetch(`${origin}${url}`)
  if (!res.ok) throw new Error('资源加载失败')
  return new Uint8Array(await res.arrayBuffer())
}

async function ensureDict(origin) {
  if (dictLoaded) return
  const DB = await fetchBin(origin, '/db/dict.bin')
  const dv = new DataView(DB.buffer, DB.byteOffset, DB.byteLength)
  let off = 0
  const m0 = dv.getUint8(off++)
  const m1 = dv.getUint8(off++)
  const m2 = dv.getUint8(off++)
  const m3 = dv.getUint8(off++)
  if (!(m0 === 0x49 && m1 === 0x50 && m2 === 0x44 && m3 === 0x43)) {
    strings = []
    triples = []
    dictLoaded = true
    return
  }
  off++
  const strCount = dv.getUint32(off, true); off += 4
  const triCount = dv.getUint32(off, true); off += 4
  strings = new Array(strCount)
  const td = new TextDecoder()
  for (let i = 0; i < strCount; i++) {
    const len = dv.getUint16(off, true); off += 2
    const bytes = DB.subarray(off, off + len); off += len
    strings[i] = td.decode(bytes)
  }
  triples = new Array(triCount)
  for (let i = 0; i < triCount; i++) {
    const a = dv.getUint16(off, true); off += 2
    const b = dv.getUint16(off, true); off += 2
    const c = dv.getUint16(off, true); off += 2
    triples[i] = [a, b, c]
  }
  dictLoaded = true
}

async function loadChunk(origin, a) {
  const cached = chunkCache.get(a)
  if (cached) return cached
  const DB = await fetchBin(origin, `/db/chunks/a${a}.bin`)
  const dv = new DataView(DB.buffer, DB.byteOffset, DB.byteLength)
  let off = 0
  const m0 = dv.getUint8(off++)
  const m1 = dv.getUint8(off++)
  const m2 = dv.getUint8(off++)
  const m3 = dv.getUint8(off++)
  if (!(m0 === 0x49 && m1 === 0x50 && m2 === 0x43 && m3 === 0x48)) {
    const arr = []
    chunkCache.set(a, arr)
    return arr
  }
  off++
  const recCount = dv.getUint32(off, true); off += 4
  const readVar = () => {
    let x = 0
    let shift = 0
    while (true) {
      const b = dv.getUint8(off++)
      x |= (b & 0x7f) << shift
      if ((b & 0x80) === 0) break
      shift += 7
    }
    return x >>> 0
  }
  const records = new Array(recCount)
  let prevStart = 0
  for (let i = 0; i < recCount; i++) {
    const delta = readVar()
    const start = (prevStart + delta) >>> 0
    const len = readVar()
    const end = (start + len) >>> 0
    const t = dv.getUint16(off, true); off += 2
    records[i] = [start, end, t]
    prevStart = start
  }
  chunkCache.set(a, records)
  return records
}

async function lookup(origin, ip) {
  await ensureDict(origin)
  const val = ipToInt(ip)
  if (val == null) return { country: null, province: null, city: null }
  const a = (val >>> 24) & 0xff
  const records = await loadChunk(origin, a)
  let lo = 0
  let hi = records.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >>> 1
    const r = records[mid]
    if (val < r[0]) hi = mid - 1; else if (val > r[1]) lo = mid + 1; else {
      const tri = triples[r[2]] || [0, 0, 0]
      const country = strings[tri[0]] || null
      const province = strings[tri[1]] || null
      const city = strings[tri[2]] || null
      return { country, province, city }
    }
  }
  return { country: null, province: null, city: null }
}

function jsonResponse(obj) {
  return new Response(JSON.stringify(obj), {
    headers: {
      'Content-Type': 'application/json; charset=utf-8',
      'Access-Control-Allow-Origin': '*',
      'Cache-Control': 'no-store',
    },
  })
}

if (typeof addEventListener === 'function') {
  addEventListener('fetch', event => {
    event.respondWith(handle(event.request))
  })
}

async function handle(req) {
  const url = new URL(req.url)
  if (url.pathname === '/api/ip') {
    const ip = url.searchParams.get('ip') || ''
    if (!ip) return jsonResponse({ ip: null, country: null, province: null, city: null })
    try {
      const origin = `${url.origin}`
      const data = await lookup(origin, ip)
      return jsonResponse({ ip, ...data })
    } catch (e) {
      return jsonResponse({ ip, country: null, province: null, city: null })
    }
  }
  return fetch(req)
}

export default async function(request) {
  return handle(request)
}
