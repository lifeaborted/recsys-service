// API-проверки рекомендаций на запущенном бэкенде (RECSYS_ADDR, по умолчанию :8080).
//   node --test tests/e2e/demo/api.test.mjs
import test from "node:test";
import assert from "node:assert/strict";

const BASE = process.env.RECSYS_ADDR?.startsWith("http")
  ? process.env.RECSYS_ADDR.replace(/\/$/, "")
  : "http://localhost:8080/api/v1";

async function req(path, opts = {}) {
  const response = await fetch(BASE + path, { ...opts, signal: AbortSignal.timeout(15000) });
  const body = await response.json().catch(() => ({}));
  return { status: response.status, body };
}

const post = (path, payload) => req(path, {
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(payload),
});

let alive = true;
try {
  const r = await req("/health");
  alive = r.status === 200;
} catch {
  alive = false;
}

test("API рекомендаций", { skip: !alive ? "бэкенд не запущен" : false }, async (t) => {
  await t.test("health и формат выдачи", async () => {
    const health = await req("/health");
    assert.equal(health.status, 200);
    assert.equal(health.body.status, "ok");

    const reco = await req("/recommendations?user_id=u1&limit=3");
    assert.equal(reco.status, 200);
    assert.ok(reco.body.request_id);
    assert.ok(reco.body.items.length > 0 && reco.body.items.length <= 3);
    for (const item of reco.body.items) {
      assert.ok(item.item_id && item.title);
      assert.ok(Number.isFinite(item.score) && item.score >= 0 && item.score <= 1);
      assert.ok(typeof item.reason === "string" && item.reason.length > 0);
    }
  });

  await t.test("валидация запросов", async () => {
    assert.equal((await req("/recommendations")).status, 400);
    assert.equal((await req("/recommendations?user_id=u1&limit=0")).status, 400);
    assert.equal((await req("/recommendations?user_id=u1&limit=abc")).status, 400);
    assert.equal((await req("/recommendations/me")).status, 401);

    const me = await req("/recommendations/me", {
      headers: { Authorization: "Bearer demo-token-u1" },
    });
    assert.equal(me.status, 200);
  });

  await t.test("профили изолированы по client_id", async () => {
    await req("/recommendations?user_id=u1&limit=1");
    await req("/recommendations?user_id=u2&limit=1");
    const [u1, u2] = await Promise.all([
      req("/debug/runs?user_id=u1"),
      req("/debug/runs?user_id=u2"),
    ]);
    assert.equal(u1.status, 200);
    assert.equal(u2.status, 200);
    assert.notEqual(u1.body.client_id, u2.body.client_id);
    const id1 = u1.body.runs?.[0]?.run_id;
    const id2 = u2.body.runs?.[0]?.run_id;
    assert.ok(id1 && id2 && id1 !== id2, "run'ы разных профилей должны различаться");
  });

  await t.test("фидбек записывается и не ломает выдачу", async () => {
    const reco = await req("/recommendations?user_id=u1&limit=1");
    const itemId = reco.body.items[0].item_id;
    const feedback = await post("/feedback", { user_id: "u1", item_id: itemId, action: "click" });
    assert.equal(feedback.status, 200);
    assert.equal(feedback.body.accepted, true);
    assert.equal((await post("/feedback", { user_id: "u1", action: "click" })).status, 400);
    const after = await req("/recommendations?user_id=u1&limit=3");
    assert.equal(after.status, 200);
  });

  await t.test("параллельный new run не дублирует id", async () => {
    const created = await Promise.all(
      Array.from({ length: 5 }, () => req("/recommendations/new?user_id=u2", { method: "POST" })),
    );
    assert.ok(created.every((r) => r.status === 200));
    const ids = created.map((r) => r.body?.run_id);
    assert.equal(new Set(ids).size, ids.length, `дубли run_id: ${ids.join(",")}`);
  });

  await t.test("параллельные запросы", async () => {
    const results = await Promise.all(
      Array.from({ length: 30 }, () => req("/recommendations?user_id=u1&limit=3")),
    );
    assert.ok(results.every((r) => r.status === 200));
  });
});
