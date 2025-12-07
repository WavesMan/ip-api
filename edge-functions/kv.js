function getKV(env) {
  if (!env) return null;
  if (env.view_stats) return env.view_stats;
  if (env.my_kv) return env.my_kv;
  if (env.MY_KV) return env.MY_KV;
  if (globalThis.my_kv) return globalThis.my_kv;
  if (globalThis.MY_KV) return globalThis.MY_KV;
  return null;
}

async function getInt(env, key) {
  const kv = getKV(env);
  if (!kv) return 0;
  const v = await kv.get(key);
  if (v == null) return 0;
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

async function putInt(env, key, n) {
  const kv = getKV(env);
  if (!kv) return n;
  await kv.put(key, String(n));
  return n;
}

function shardCount() {
  return 16;
}

function randomShard() {
  return Math.floor(Math.random() * shardCount());
}

function fmtDateUTC(d) {
  const y = d.getUTCFullYear();
  const m = String(d.getUTCMonth() + 1).padStart(2, "0");
  const day = String(d.getUTCDate()).padStart(2, "0");
  return `${y}${m}${day}`;
}

function keyTotalShard(s) {
  return `req:total:${s}`;
}

function keyDayShard(dateStr, s) {
  return `req:${dateStr}:${s}`;
}

export async function incrTotal(env) {
  const s = randomShard();
  const key = keyTotalShard(s);
  const cur = await getInt(env, key);
  return await putInt(env, key, cur + 1);
}

export async function incrToday(env) {
  const s = randomShard();
  const dateStr = fmtDateUTC(new Date());
  const key = keyDayShard(dateStr, s);
  const cur = await getInt(env, key);
  return await putInt(env, key, cur + 1);
}

export async function sumTotal(env) {
  let sum = 0;
  for (let s = 0; s < shardCount(); s++) {
    sum += await getInt(env, keyTotalShard(s));
  }
  return sum;
}

export async function sumToday(env) {
  const dateStr = fmtDateUTC(new Date());
  let sum = 0;
  for (let s = 0; s < shardCount(); s++) {
    sum += await getInt(env, keyDayShard(dateStr, s));
  }
  return sum;
}
