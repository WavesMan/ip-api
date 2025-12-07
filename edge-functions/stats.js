import { sumTotal, sumToday } from "./kv.js";

export async function onRequest({ request, env }) {
  const total = await sumTotal(env);
  const today = await sumToday(env);
  return new Response(JSON.stringify({ total, today }), {
    status: 200,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}

export default async function handler(req) {
  return new Response(JSON.stringify({ total: 0, today: 0 }), {
    status: 200,
    headers: { "content-type": "application/json; charset=utf-8" },
  });
}
