import test from "node:test";
import assert from "node:assert/strict";

const base = (process.env.RECSYS_E2E_API ?? "http://127.0.0.1:8080").replace(/\/$/, "");

async function request(path, { token, body, requestID } = {}) {
  const response = await fetch(base + path, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      ...(token ? { Authorization: "Bearer " + token } : {}),
      ...(requestID ? { "X-Request-ID": requestID } : {}),
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal: AbortSignal.timeout(10_000),
  });
  const text = await response.text();
  const data = response.headers.get("content-type")?.includes("json") ? JSON.parse(text) : text;
  return { status: response.status, id: response.headers.get("x-request-id"), data };
}

test("OpenAPI, feed metadata and isolated purchase-to-upgrade flow", async () => {
  const health = await request("/api/v1/health");
  assert.equal(health.status, 200);
  assert.equal(health.data.status, "ok");
  const ui = await request("/swagger/");
  assert.equal(ui.status, 200);
  assert.match(ui.data, /\/swagger\/openapi\.yaml/);
  const spec = await request("/swagger/openapi.yaml");
  assert.equal(spec.status, 200);
  assert.match(spec.data, /operationId: getCurrentRecommendations/);

  const unauth = await request("/api/v1/recommendations/me", { requestID: "e2e-probe" });
  assert.equal(unauth.status, 401);
  assert.equal(unauth.id, "e2e-probe");
  assert.equal(unauth.data.request_id, unauth.id);

  const register = await request("/api/v1/auth/register", {
    body: { login: "integration-" + crypto.randomUUID(), password: "synthetic-password", client_group: "B2C" },
  });
  assert.equal(register.status, 201);
  assert.ok(register.data.access_token);
  const login = async (name, password) => {
    const result = await request("/api/v1/auth/login", { body: { login: name, password } });
    assert.equal(result.status, 200);
    assert.ok(result.data.access_token);
    return result.data.access_token;
  };
  const first = await login("demo-b2b", process.env.DEMO_B2B_PASSWORD ?? "demo-b2b-pass");
  const second = await login("demo-b2c", process.env.DEMO_B2C_PASSWORD ?? "demo-b2c-pass");
  const me = await request("/api/v1/users/me", { token: first });
  const other = await request("/api/v1/users/me", { token: second });
  assert.equal(me.status, 200);
  assert.equal(other.status, 200);
  assert.notEqual(me.data.profile_id, other.data.profile_id);

  const catalog = await request("/api/v1/products?limit=100", { token: first });
  assert.equal(catalog.status, 200);
  assert.equal(catalog.data.feed_generated_at, health.data.feed_generated_at);
  assert.match(catalog.data.feed_generated_at, /^2026-08-31T/);
  assert.equal(catalog.data.limit, 100);
  assert.ok(catalog.data.items.some((item) => item.item_id === "DEMO-BASE" && item.available));

  const rejected = await request("/api/v1/purchases", {
    token: first, body: { item_id: "DEMO-BASE", profile_id: other.data.profile_id },
  });
  assert.equal(rejected.status, 400);
  const spoofedQuery = await request("/api/v1/recommendations/me?client_id=DEMO-NEW", { token: first });
  assert.equal(spoofedQuery.status, 400);

  const recommend = async (token, limit = 36) => {
    const result = await request(`/api/v1/recommendations/me?limit=${limit}`, { token });
    assert.equal(result.status, 200);
    const body = result.data;
    assert.equal(body.request_id, result.id);
    assert.equal(body.snapshot_id, "demo-v1");
    const nodes = new Set(body.graph.nodes.map((node) => node.id));
    const edges = new Map(body.graph.edges.map((edge) => [edge.id, edge]));
    for (const edge of edges.values()) {
      assert.ok(nodes.has(edge.from) && nodes.has(edge.to) && edge.source);
      assert.equal(edge.snapshot_id, body.snapshot_id);
    }
    assert.ok(body.items.length <= limit);
    for (const item of body.items) {
      assert.notEqual(item.recommendation_kind, "renewal");
      assert.ok(item.evidence_ids.every((id) => edges.has(id)));
    }
    return body;
  };
  const isolated = (await recommend(second)).items.map((item) => item.item_id);
  const purchase = async (item_id) => {
    const result = await request("/api/v1/purchases", { token: first, body: { item_id } });
    assert.equal(result.status, 201);
    assert.equal(result.data.item_id, item_id);
  };

  await purchase("DEMO-BASE");
  const before = await recommend(first);
  const upgrade = before.items.find((item) => item.item_id === "DEMO-UPGRADE");
  assert.equal(upgrade?.recommendation_kind, "upgrade");
  assert.ok(upgrade.evidence_ids.some((id) => before.graph.edges.some((edge) =>
    edge.id === id && edge.from === "item:DEMO-BASE" && edge.type === "upgrade")));
  assert.ok(before.items.some((item) => item.item_id === "DEMO-COMPLEMENT" && item.recommendation_kind === "complement"));
  const limited = await recommend(first, 1);
  assert.equal(limited.items.length, 1);
  assert.equal(limited.partial_reason, null);
  await purchase("DEMO-UPGRADE");
  const partial = await recommend(first);
  assert.ok(partial.items.some((item) => item.item_id === "DEMO-COMPLEMENT"));
  assert.ok(!partial.items.some((item) => item.item_id === "DEMO-UPGRADE"));
  assert.ok(partial.items.length > 0 && partial.items.length < 3);
  assert.equal(partial.partial_reason, "insufficient_eligible_products");
  assert.deepEqual((await recommend(second)).items.map((item) => item.item_id), isolated);
});
