// Браузерный тест демо-экрана (05-DEMO, п. «Проверка и передача»).
// Требует запущенный Vite: npm run dev. HTTP подменён синтетикой — Go API не нужен.
//   npm run test:e2e:demo
import test from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { chromium } from "playwright-core";

const SCREEN = "http://localhost:5173/#/demo";
const CHROME_CANDIDATES = [
  process.env.CHROME_PATH,
  "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
  "C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
  "/usr/bin/google-chrome",
  "/usr/bin/chromium",
].filter(Boolean);

function chromePath() {
  return CHROME_CANDIDATES.find((path) => existsSync(path));
}

async function serverUp() {
  try {
    const response = await fetch("http://localhost:5173/", { signal: AbortSignal.timeout(2000) });
    return response.ok;
  } catch {
    return false;
  }
}

const up = await serverUp();
const executablePath = chromePath();

test("демо-экран: карточки, варианты, ошибка и смена профиля", { skip: !up || !executablePath ? "нужны Vite и Chrome" : false }, async (t) => {
  const browser = await chromium.launch({ executablePath, headless: true, args: ["--no-sandbox"] });
  t.after(() => browser.close());

  const page = await browser.newPage();
  const consoleErrors = [];
  page.on("console", (message) => {
    if (message.type() === "error" && !message.text().includes("404")) consoleErrors.push(message.text());
  });
  await page.goto(SCREEN);
  await page.evaluate(() => localStorage.clear());
  await page.reload();
  await page.getByText("Открыть карточку").first().waitFor();

  const profile = page.locator("select").nth(0);
  const scenario = page.locator("select").nth(2);

  await t.test("полная выдача: три карточки и DEMO-RENEW", async () => {
    await profile.selectOption("u1");
    await scenario.selectOption("full");
    await page.waitForFunction(() => document.querySelectorAll(".demo-card").length === 3);
    const titles = await page.locator(".demo-card__title").allTextContents();
    assert.equal(titles.length, 3);
    assert.ok(titles.includes("Продление на 12 месяцев"));

    await page.getByRole("button", { name: "Сравнить варианты" }).first().click();
    const variants = await page.locator(".demo-variant").allTextContents();
    assert.ok(variants.some((text) => text.includes("12 мес.")));
    assert.ok(variants.some((text) => text.includes("24 мес.")));
    assert.ok(variants.some((text) => text.includes("Срок окончания неизвестен")));
  });

  await t.test("объяснение раскрывает только связанное ребро", async () => {
    await page.getByRole("button", { name: "Почему рекомендуем" }).first().click();
    const reason = await page.locator(".demo-card__source").first().textContent();
    assert.match(reason, /item:DEMO-BASE → item:DEMO-RENEW-12/);
    assert.match(reason, /synthetic-rule-1/);
  });

  await t.test("частичная и пустая выдача", async () => {
    await scenario.selectOption("partial");
    await page.getByText("insufficient_eligible_products").waitFor();
    assert.equal(await page.locator(".demo-card").count(), 1);

    await scenario.selectOption("empty");
    await page.getByText("Нет рекомендаций для этого профиля.").waitFor();
    assert.equal(await page.locator(".demo-card").count(), 0);
  });

  await t.test("ошибка показывает request_id и убирает карточки", async () => {
    await scenario.selectOption("error");
    await page.getByText("model_unavailable").waitFor();
    const banner = await page.locator(".demo-banner_error").textContent();
    assert.match(banner, /demo-error-1/);
    assert.equal(await page.locator(".demo-card").count(), 0);
  });

  await t.test("медленный ответ старой сессии игнорируется", async () => {
    await scenario.selectOption("slow");
    await profile.selectOption("u2");
    await scenario.selectOption("auto");
    await page.getByText("insufficient_eligible_products").waitFor();
    await page.waitForTimeout(1900);
    const status = await page.locator(".demo-status").first().textContent();
    assert.match(status, /u2/);
    assert.match(status, /demo-1/);
    assert.doesNotMatch(status, /demo-full-1/);
  });

  await t.test("консоль без ошибок приложения", async () => {
    assert.deepEqual(consoleErrors, []);
  });
});
