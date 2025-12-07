import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "..");
const outDict = path.join(root, "edge-functions", "dict.js");
const chunksDir = path.join(root, "edge-functions", "chunks");
const sourceUrl = "https://raw.githubusercontent.com/lionsoul2014/ip2region/master/data/ipv4_source.txt";

function ipToInt(ip) {
  const p = ip.split(".").map(Number);
  return ((p[0] << 24) >>> 0) + (p[1] << 16) + (p[2] << 8) + p[3];
}

function norm(s) {
  if (s === undefined || s === null) return "";
  if (s === "0") return "";
  return s;
}

async function download() {
  const res = await fetch(sourceUrl);
  if (!res.ok) throw new Error(`download failed: ${res.status}`);
  return await res.text();
}

function build(lines) {
  const triplesMap = new Map();
  const triples = [];
  const stringMap = new Map();
  const strings = [];
  const records = [];
  let last = null;
  for (const line of lines) {
    if (!line || line.startsWith("#")) continue;
    const parts = line.split("|");
    if (parts.length < 6) continue;
    const s = ipToInt(parts[0]);
    const e = ipToInt(parts[1]);
    const country = norm(parts[2]);
    const province = norm(parts[3]);
    const city = norm(parts[4]);
    const key = `${country}|${province}|${city}`;
    let triIdx = triplesMap.get(key);
    if (triIdx === undefined) {
      const getIdx = (str) => {
        let idx = stringMap.get(str);
        if (idx === undefined) {
          idx = strings.length;
          strings.push(str);
          stringMap.set(str, idx);
        }
        return idx;
      };
      const a = getIdx(country);
      const b = getIdx(province);
      const c = getIdx(city);
      triIdx = triples.length;
      triples.push([a, b, c]);
      triplesMap.set(key, triIdx);
    }
    if (last && last[2] === triIdx && last[1] + 1 === s) {
      last[1] = e;
    } else {
      last = [s, e, triIdx];
      records.push(last);
    }
  }
  records.sort((x, y) => x[0] - y[0]);
  return { strings, triples, records };
}

function encodeDict(strings, triples) {
  const enc = new TextEncoder();
  const strBufs = strings.map((s) => enc.encode(s));
  const bytes = [];
  const push8 = (v) => bytes.push(v & 0xff);
  const push16 = (v) => { bytes.push(v & 0xff, (v >>> 8) & 0xff); };
  const push32 = (v) => { bytes.push(v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff); };
  for (const m of [0x49,0x50,0x44,0x43]) push8(m);
  push8(1);
  push32(strings.length);
  push32(triples.length);
  for (let i = 0; i < strings.length; i++) {
    const b = strBufs[i];
    push16(b.length);
    for (let j = 0; j < b.length; j++) push8(b[j]);
  }
  for (const t of triples) {
    push16(t[0]);
    push16(t[1]);
    push16(t[2]);
  }
  return new Uint8Array(bytes);
}

function splitToChunks(records) {
  const buckets = Array.from({ length: 256 }, () => []);
  const rangeStart = (a) => (a << 24) >>> 0;
  const rangeEnd = (a) => (((a << 24) >>> 0) + 0x00FFFFFF) >>> 0;
  for (const r of records) {
    const s = r[0];
    const e = r[1];
    const tri = r[2];
    const aStart = s >>> 24;
    const aEnd = e >>> 24;
    for (let a = aStart; a <= aEnd; a++) {
      const rs = rangeStart(a);
      const re = rangeEnd(a);
      const subS = s > rs ? s : rs;
      const subE = e < re ? e : re;
      if (subS <= subE) buckets[a].push([subS, subE, tri]);
    }
  }
  for (let a = 0; a < 256; a++) {
    const arr = buckets[a];
    arr.sort((x, y) => x[0] - y[0]);
    const merged = [];
    let last = null;
    for (const it of arr) {
      if (last && last[2] === it[2] && last[1] + 1 === it[0]) {
        last[1] = it[1];
      } else {
        last = [it[0], it[1], it[2]];
        merged.push(last);
      }
    }
    buckets[a] = merged;
  }
  return buckets;
}

function encodeChunk(records) {
  const bytes = [];
  const push8 = (v) => bytes.push(v & 0xff);
  const push16 = (v) => { bytes.push(v & 0xff, (v >>> 8) & 0xff); };
  const push32 = (v) => { bytes.push(v & 0xff, (v >>> 8) & 0xff, (v >>> 16) & 0xff, (v >>> 24) & 0xff); };
  const pushVar = (v) => { let x = v >>> 0; while (x >= 0x80) { bytes.push((x & 0x7f) | 0x80); x >>>= 7; } bytes.push(x & 0x7f); };
  for (const m of [0x49,0x50,0x43,0x48]) push8(m);
  push8(2);
  push32(records.length);
  let prevStart = 0;
  for (const r of records) {
    const start = r[0];
    const len = (r[1] - r[0]) >>> 0;
    const delta = (start - prevStart) >>> 0;
    pushVar(delta);
    pushVar(len);
    push16(r[2]);
    prevStart = start;
  }
  return new Uint8Array(bytes);
}

async function main() {
  const txt = await download();
  const lines = txt.split(/\r?\n/);
  const { strings, triples, records } = build(lines);
  const dictBuf = encodeDict(strings, triples);
  fs.mkdirSync(path.join(root, "edge-functions"), { recursive: true });
  fs.mkdirSync(chunksDir, { recursive: true });
  fs.writeFileSync(outDict, `export const DICT = new Uint8Array([${Array.from(dictBuf).join(",")}]);\n`);
  const buckets = splitToChunks(records);
  let total = 0;
  for (let a = 0; a < 256; a++) {
    const buf = encodeChunk(buckets[a]);
    total += buf.length;
    const file = path.join(chunksDir, `a${a}.js`);
    fs.writeFileSync(file, `export const CH = new Uint8Array([${Array.from(buf).join(",")}]);\n`);
  }
  console.log(`generated ${outDict} (${dictBuf.length} bytes), chunks total ${total} bytes`);
}

main().catch((e) => {
  console.error(e.message || String(e));
  process.exit(1);
});
