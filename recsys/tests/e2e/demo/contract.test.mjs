import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const root = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "..");
const examples = join(root, "api", "examples");

function load(name) {
  return JSON.parse(readFileSync(join(examples, name), "utf8"));
}

const full = load("recommendations.full.json");
const partial = load("recommendations.partial.json");
const empty = load("recommendations.empty.json");
const failed = load("recommendations.error.json");
const verified = load("recommendations.verified.json");

function assertEdgesResolvable(payload, label) {
  const nodes = new Set(payload.graph.nodes.map((node) => node.id));
  const edges = new Set(payload.graph.edges.map((edge) => edge.id));

  for (const edge of payload.graph.edges) {
    assert.ok(nodes.has(edge.from), `${label}: начало ребра ${edge.id} отсутствует в nodes`);
    assert.ok(nodes.has(edge.to), `${label}: конец ребра ${edge.id} отсутствует в nodes`);
    assert.equal(edge.snapshot_id, payload.snapshot_id, `${label}: snapshot_id ребра не совпадает`);
  }
  for (const item of payload.items) {
    for (const id of item.evidence_ids) {
      assert.ok(edges.has(id), `${label}: evidence_id ${id} не разрешается в графе`);
    }
  }
}

function assertItemShape(item, label) {
  for (const field of [
    "item_id", "sku", "recommendation_kind", "family_id",
    "title", "explanation", "variants", "evidence_ids",
  ]) {
    assert.ok(Object.hasOwn(item, field), `${label}: отсутствует поле ${field}`);
  }
  assert.ok(Array.isArray(item.variants), `${label}: variants должен быть массивом`);
  assert.ok(Array.isArray(item.evidence_ids), `${label}: evidence_ids должен быть массивом`);
  if (item.url) {
    assert.match(item.url, /^https?:\/\//, `${label}: ссылка должна быть абсолютной`);
  }
}

test("примеры читаются и имеют базовую форму", () => {
  for (const [name, payload] of Object.entries({ full, partial, empty })) {
    assert.ok(payload.request_id, `${name}: нет request_id`);
    assert.ok(payload.snapshot_id, `${name}: нет snapshot_id`);
    assert.ok(Array.isArray(payload.items), `${name}: items должен быть массивом`);
    assert.ok(payload.graph && Array.isArray(payload.graph.nodes) && Array.isArray(payload.graph.edges));
    assert.ok(!Object.hasOwn(payload, "items") || payload.items.length <= 3, `${name}: больше трёх карточек`);
  }
});

test("полная выдача содержит ровно три верхнеуровневые карточки", () => {
  assert.equal(full.items.length, 3);
  assert.equal(full.partial_reason, null);
  full.items.forEach((item, index) => assertItemShape(item, `full.items[${index}]`));
  assertEdgesResolvable(full, "full");
});

test("частичная выдача помечена причиной и содержит одну карточку", () => {
  assert.equal(partial.items.length, 1);
  assert.equal(partial.partial_reason, "insufficient_eligible_products");
  assertItemShape(partial.items[0], "partial.items[0]");
  assertEdgesResolvable(partial, "partial");
});

test("пустая выдача представляет пустые списки и null", () => {
  assert.deepEqual(empty.items, []);
  assert.deepEqual(empty.graph.nodes, []);
  assert.deepEqual(empty.graph.edges, []);
  assert.equal(empty.partial_reason, null);
});

test("четвёртой верхнеуровневой карточки нет ни в одном примере", () => {
  for (const [name, payload] of Object.entries({ full, partial, empty })) {
    assert.ok(payload.items.length < 4, `${name}: есть четвёртая карточка`);
  }
});

test("DEMO-RENEW показывает два варианта с разными сроками", () => {
  const item = full.items.find((entry) => entry.item_id === "DEMO-RENEW-12");
  assert.ok(item, "в полной выдаче нет DEMO-RENEW-12");
  assert.equal(item.recommendation_kind, "renewal");
  assert.equal(item.variants.length, 2);

  const terms = item.variants.map((variant) =>
    variant.differences.find((difference) => difference.field === "term")?.value);
  assert.deepEqual(terms.sort((a, b) => a - b), [12, 24]);
  assert.equal(new Set(terms).size, 2, "варианты должны отличаться сроком");
  assertEdgesResolvable(full, "full-renew");
});

test("неизвестный срок окончания сохраняется как null", () => {
  const item = full.items.find((entry) => entry.item_id === "DEMO-RENEW-12");
  const unknown = item.variants.some((variant) =>
    variant.differences.some((difference) => difference.field === "expires_at" && difference.value === null));
  assert.ok(unknown, "состояние «срок неизвестен» потеряно");
});

test("ошибка содержит request_id и код", () => {
  assert.ok(failed.request_id, "у ошибки нет request_id");
  assert.equal(typeof failed.error.code, "string");
  assert.equal(typeof failed.error.message, "string");
  assert.ok(!failed.items, "ошибка не должна содержать items");
});

test("пример с проверкой совместимости отделяет подтверждённое от «требует проверки»", () => {
  assert.equal(verified.items.length, 2);
  assert.ok(verified.items.length < 4, "в новом примере не должно быть четвёртой карточки");
  assert.equal(verified.partial_reason, null);
  verified.items.forEach((item, index) => assertItemShape(item, `verified.items[${index}]`));

  const confirmed = verified.items.find((item) => item.verification === "confirmed");
  const requiresCheck = verified.items.find((item) => item.verification === "requires_check");
  assert.ok(confirmed, "нет подтверждённой карточки");
  assert.ok(requiresCheck, "нет карточки «требует проверки»");
  assert.notEqual(confirmed.verification, requiresCheck.verification);
  assert.ok(
    confirmed.recommendation_kind === "upgrade" || confirmed.recommendation_kind === "complement",
    "подтверждённая карточка должна быть апгрейдом или дополнением",
  );
  assert.ok(
    !verified.items.some((item) => item.recommendation_kind === "renewal"),
    "в примере проверки не должно быть продления",
  );
  assertEdgesResolvable(verified, "verified");
});

test("новый пример с проверкой не ломает старый пример на три карточки", () => {
  assert.equal(full.items.length, 3);
  assert.ok(!full.items.some((item) => item.verification === "requires_check"));
  assertEdgesResolvable(full, "full-after-verified");
});
