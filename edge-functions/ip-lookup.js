let dictLoaded = false;
let strings = null;
let triples = null;
const chunkCache = new Map();
let cacheStats = { value: null, ts: 0 };
import { incrTotal, incrToday } from "./kv.js";

function ipToInt(ip) {
  const p = ip.split(".");
  if (p.length !== 4) return null;
  for (let i = 0; i < 4; i++) {
    const n = Number(p[i]);
    if (!Number.isInteger(n) || n < 0 || n > 255) return null;
  }
  return ((Number(p[0]) << 24) >>> 0) + (Number(p[1]) << 16) + (Number(p[2]) << 8) + Number(p[3]);
}

async function ensureDict() {
  if (dictLoaded) return;
  const mod = await import("./dict.js");
  const DB = mod.DICT;
  const dv = new DataView(DB.buffer, DB.byteOffset, DB.byteLength);
  let off = 0;
  const m0 = dv.getUint8(off++);
  const m1 = dv.getUint8(off++);
  const m2 = dv.getUint8(off++);
  const m3 = dv.getUint8(off++);
  if (!(m0 === 0x49 && m1 === 0x50 && m2 === 0x44 && m3 === 0x43)) {
    strings = [];
    triples = [];
    dictLoaded = true;
    return;
  }
  const ver = dv.getUint8(off++);
  const strCount = dv.getUint32(off, true); off += 4;
  const triCount = dv.getUint32(off, true); off += 4;
  strings = new Array(strCount);
  for (let i = 0; i < strCount; i++) {
    const len = dv.getUint16(off, true); off += 2;
    const bytes = DB.subarray(off, off + len); off += len;
    strings[i] = new TextDecoder().decode(bytes);
  }
  triples = new Array(triCount);
  for (let i = 0; i < triCount; i++) {
    const a = dv.getUint16(off, true); off += 2;
    const b = dv.getUint16(off, true); off += 2;
    const c = dv.getUint16(off, true); off += 2;
    triples[i] = [a, b, c];
  }
  dictLoaded = true;
}

async function loadChunk(a) {
  const cached = chunkCache.get(a);
  if (cached) return cached;
  const mod = await import(`./chunks/a${a}.js`);
  const DB = mod.CH;
  const dv = new DataView(DB.buffer, DB.byteOffset, DB.byteLength);
  let off = 0;
  const m0 = dv.getUint8(off++);
  const m1 = dv.getUint8(off++);
  const m2 = dv.getUint8(off++);
  const m3 = dv.getUint8(off++);
  if (!(m0 === 0x49 && m1 === 0x50 && m2 === 0x43 && m3 === 0x48)) {
    const arr = [];
    chunkCache.set(a, arr);
    return arr;
  }
  const ver = dv.getUint8(off++);
  const recCount = dv.getUint32(off, true); off += 4;
  const readVar = () => {
    let x = 0;
    let shift = 0;
    while (true) {
      const b = dv.getUint8(off++);
      x |= (b & 0x7f) << shift;
      if ((b & 0x80) === 0) break;
      shift += 7;
    }
    return x >>> 0;
  };
  const records = new Array(recCount);
  let prevStart = 0;
  for (let i = 0; i < recCount; i++) {
    const delta = readVar();
    const start = (prevStart + delta) >>> 0;
    const len = readVar();
    const end = (start + len) >>> 0;
    const t = dv.getUint16(off, true); off += 2;
    records[i] = [start, end, t];
    prevStart = start;
  }
  chunkCache.set(a, records);
  return records;
}

async function lookup(ip) {
  await ensureDict();
  const val = ipToInt(ip);
  if (val == null) return { country: null, province: null, city: null };
  const a = (val >>> 24) & 0xff;
  const records = await loadChunk(a);
  let lo = 0;
  let hi = records.length - 1;
  while (lo <= hi) {
    const mid = (lo + hi) >>> 1;
    const r = records[mid];
    if (val < r[0]) hi = mid - 1; else if (val > r[1]) lo = mid + 1; else {
      const tri = triples[r[2]] || [0, 0, 0];
      const country = strings[tri[0]] || null;
      const province = strings[tri[1]] || null;
      const city = strings[tri[2]] || null;
      return { country, province, city };
    }
  }
  return { country: null, province: null, city: null };
}

function getClientIP(req) {
  const url = new URL(req.url);
  const qp = url.searchParams.get("ip");
  if (qp) return qp;
  const h = req.headers;
  const h1 = h.get?.("x-forwarded-for") || h["x-forwarded-for"];
  const h2 = h.get?.("cf-connecting-ip") || h["cf-connecting-ip"];
  const h3 = h.get?.("x-real-ip") || h["x-real-ip"];
  const h4 = h.get?.("x-client-ip") || h["x-client-ip"];
  const h5 = h.get?.("x-edge-client-ip") || h["x-edge-client-ip"] || h.get?.("x-edgeone-ip") || h["x-edgeone-ip"];
  const fwd = h.get?.("forwarded") || h["forwarded"];
  let fwdIp = "";
  if (typeof fwd === "string") {
    const m = /for="?([0-9.]+)"?/i.exec(fwd);
    if (m) fwdIp = m[1];
  }
  const cand = h1?.split(",")[0]?.trim() || fwdIp || h2 || h3 || h4 || h5;
  return cand || "";
}

export default async function handler(req) {
  const ip = getClientIP(req);
  const res = await lookup(ip);
  return new Response(JSON.stringify({ ip, ...res }), {
    status: 200,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}

export async function onRequest({ request, env }) {
  const ip = getClientIP(request);
  const res = await lookup(ip);
  try {
    await Promise.all([incrTotal(env), incrToday(env)]);
  } catch {}
  return new Response(JSON.stringify({ ip, ...res }), {
    status: 200,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}
