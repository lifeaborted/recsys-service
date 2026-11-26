/**
 * Достаёт из pg_dump только DDL (до первого COPY и после последнего \) и
 * сохраняет его как миграцию 0001_init.sql. Данные остаются в recommendations.sql.
 *   node tools/extract-schema.mjs
 */
import { createReadStream, mkdirSync, writeFileSync } from "node:fs";
import { createInterface } from "node:readline";

const dump = new URL("../recommendations.sql", import.meta.url);
const out = new URL("../migrations/0001_init.sql", import.meta.url);

const header = [];
let footer = [];
let mode = "header"; // header -> data -> between

const rl = createInterface({ input: createReadStream(dump), crlfDelay: Infinity });
for await (const line of rl) {
  const isCopy = line.startsWith('COPY "recommendations"');
  if (mode === "header") {
    if (isCopy) mode = "data";
    else header.push(line);
  } else if (mode === "data") {
    if (line === "\\.") mode = "between";
  } else if (isCopy) {
    mode = "data";
    footer = [];
  } else {
    footer.push(line);
  }
}
rl.close();

const clean = (lines) =>
  lines
    .filter((line) => !/^\\(un)?restrict\b/.test(line))
    .join("\n")
    .replace(/CREATE SCHEMA "recommendations";/, 'CREATE SCHEMA IF NOT EXISTS "recommendations";')
    .replace(/\n{3,}/g, "\n\n")
    .trim();

mkdirSync(new URL("../migrations/", import.meta.url), { recursive: true });
const ddl = clean([...header, ...footer]);
writeFileSync(out, ddl + "\n", "utf8");
console.log(`OK: ${out.pathname} (${ddl.length} bytes)`);
