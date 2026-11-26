/**
 * Извлекает статическую разметку исходного снимка lk.softline.ru в
 * src/data/chrome.ts. Выполнять из корня проекта:
 *   node tools/gen-fragments.mjs
 *
 * Исходный файл должен лежать на E:\ (как его прислали). Скрипт не трогает
 * React-логику — только константы с вёрсткой шапки, сайдбара и виджетов.
 */
import { readFileSync, writeFileSync } from "node:fs";

const source = "E:\\Главная.html";
const html = readFileSync(source, "utf8");

function balancedDivEnd(text, start) {
  let depth = 0;
  for (const match of text.matchAll(/<div\b|<\/div>/g)) {
    if (match.index < start) continue;
    if (match[0] === "</div>") {
      depth -= 1;
      if (depth === 0) return match.index + match[0].length;
    } else {
      depth += 1;
    }
  }
  throw new Error("Не найден закрывающий div");
}

function divBefore(text, marker) {
  const at = text.indexOf(marker);
  if (at < 0) throw new Error(`Маркер не найден: ${marker}`);
  const start = text.lastIndexOf("<div", at);
  return { start, end: balancedDivEnd(text, start) };
}

function slice(text, from, to) {
  const start = text.indexOf(from);
  if (start < 0) throw new Error(`Маркер не найден: ${from}`);
  const end = text.indexOf(to, start);
  if (end < 0) throw new Error(`Конец не найден: ${to}`);
  return text.slice(start, end + to.length);
}

function localize(value) {
  const map = [
    ["https://lk.softline.ru/fulfillment", "#/keys"],
    ["https://lk.softline.ru/trial-license", "#/trials"],
    ["https://lk.softline.ru/notifications", "#/notifications"],
    ["https://lk.softline.ru/invoices", "#/invoices"],
    ["https://lk.softline.ru/contacts", "#/contacts"],
    ["https://lk.softline.ru/products", "#/catalog"],
    ["https://lk.softline.ru/profile", "#/profile"],
    ["https://login.softline.ru/profile", "#/profile"],
    ["https://lk.softline.ru/", "#/overview"],
    ["https://store.softline.ru/", "#/overview"],
  ];
  for (const [from, to] of map) value = value.split(from).join(to);

  value = value
    .replace(/\.\/Главная_files\/logo_resize_logo40x200_([a-f0-9]+)(?!\.png)/g,
      "/assets/logo_resize_logo40x200_$1.png")
    .replace(/\.\/Главная_files\//g, "/assets/");
  return value;
}

const header = localize(slice(html, '<header class="b-store-header">', "</header>"));

const layoutAt = html.indexOf('class="layout main-container"');
const navAt = html.indexOf('<nav class="sidebar">', layoutAt);
const navEnd = html.indexOf("</nav>", navAt) + "</nav>".length;
const sidebar = localize(html.slice(navAt, navEnd));

const chunks = {
  ALERT_HTML: localize(html.slice(...Object.values(divBefore(html, 'class="alert-wrapper"')))),
  INFO_WIDGETS_HTML: localize(html.slice(...Object.values(divBefore(html, 'id="widget-slider"')))),
  PROMO_HTML: localize(html.slice(...Object.values(divBefore(html, 'id="promotion-slider"')))),
  POPULAR_HTML: localize(html.slice(...Object.values(divBefore(html, 'id="popular-solutions-widget"')))),
  EVENTS_HTML: localize(html.slice(...Object.values(divBefore(html, 'id="events-widget"')))),
};

const body = [
  "/* Сгенерировано из снимка lk.softline.ru — не редактировать вручную.",
  "   Пересоздание: node tools/gen-fragments.mjs */",
  `export const HEADER_HTML = ${JSON.stringify(header)};`,
  `export const SIDEBAR_HTML = ${JSON.stringify(sidebar)};`,
  ...Object.entries(chunks).map(([name, value]) => `export const ${name} = ${JSON.stringify(value)};`),
  "",
].join("\n");

writeFileSync("src/data/chrome.ts", body, "utf8");
console.log("OK: src/data/chrome.ts");
