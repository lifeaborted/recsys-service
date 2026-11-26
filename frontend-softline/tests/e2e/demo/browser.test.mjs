// Браузерный тест демо-экрана (05-DEMO, п. «Проверка и передача»).
// Требует запущенный Vite: npm run dev. HTTP подменён синтетикой — Go API не нужен.
// Адрес экрана: FRONTEND_URL (по умолчанию http://localhost:5173).
// Браузер: CHROME_PATH или любой chrome/chromium/msedge в PATH.
//   npm run test:e2e:demo
import test from "node:test";
import assert from "node:assert/strict";
import { existsSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { chromium } from "playwright-core";

const ORIGIN = (process.env.FRONTEND_URL ?? "http://localhost:5173").replace(/\/$/, "");
const SCREEN = `${ORIGIN}/#/demo`;

function findBrowser() {
  if (process.env.CHROME_PATH) return process.env.CHROME_PATH;

  const names = process.platform === "win32"
    ? ["chrome.exe", "msedge.exe", "chromium.exe"]
    : ["google-chrome", "chromium", "chromium-browser", "microsoft-edge"];
  const finder = process.platform === "win32" ? "where.exe" : "which";

  for (const name of names) {
    try {
      const output = execFileSync(finder, [name], {
        encoding: "utf8",
        stdio: ["ignore", "pipe", "ignore"],
      });
      const candidate = output.split(/\r?\n/).map((line) => line.trim()).find(Boolean);
      if (candidate && existsSync(candidate)) return candidate;
    } catch {
      // браузер с таким именем не найден — пробуем следующий
    }
  }

  if (process.platform === "win32") {
    // Chrome/Edge не обязательно лежат в PATH; спросим реестр Windows.
    for (const key of [
      "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\App Paths\\chrome.exe",
      "HKLM\\SOFTWARE\\WOW6432Node\\Microsoft\\Windows\\CurrentVersion\\App Paths\\chrome.exe",
      "HKCU\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\App Paths\\chrome.exe",
      "HKLM\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\App Paths\\msedge.exe",
      "HKCU\\SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\App Paths\\msedge.exe",
    ]) {
      try {
        const output = execFileSync("reg.exe", ["query", key, "/ve"], {
          encoding: "utf8",
          stdio: ["ignore", "pipe", "ignore"],
        });
        for (const line of output.split(/\r?\n/)) {
          const candidate = line.split(/\s+REG_SZ\s+/)[1]?.trim();
          if (candidate && existsSync(candidate)) return candidate;
        }
      } catch {
        // ключ отсутствует — пробуем следующий
      }
    }
  }

  try {
    const candidate = chromium.executablePath();
    if (candidate && existsSync(candidate)) return candidate;
  } catch {
    // playwright-core не скачивал браузер — значит используем системный
  }
  return undefined;
}

async function serverUp() {
  try {
    const response = await fetch(ORIGIN, { signal: AbortSignal.timeout(2000) });
    return response.ok;
  } catch {
    return false;
  }
}

const up = await serverUp();
const executablePath = findBrowser();
let apiUp = false;
try {
  const response = await fetch((process.env.RECSYS_E2E_API ?? "http://localhost:8080") + "/api/v1/health", { signal: AbortSignal.timeout(2000) });
  apiUp = response.ok;
} catch {
  apiUp = false;
}

test("демо-экран: карточки, варианты, ошибка и смена профиля", { skip: !up || !executablePath ? "нужны Vite и браузер в PATH (или CHROME_PATH)" : false }, async (t) => {
  const browser = await chromium.launch({ executablePath, headless: true, args: ["--no-sandbox"] });
  t.after(() => browser.close());

  const page = await browser.newPage();
  const consoleErrors = [];
  page.on("console", (message) => {
    // 404 (favicon) и 401 в момент переключения транспорта — ожидаемые переходы.
    if (message.type() === "error" && !message.text().includes("404") && !message.text().includes("401")) {
      consoleErrors.push(message.text());
    }
  });
  await page.goto(SCREEN);
  // Тест экрана работает на подмене транспорта: включаем mock явно.
  await page.evaluate(() => {
    localStorage.clear();
    localStorage.setItem("lk-demo.session.v2", JSON.stringify({
      transport: "mock", scenario: "auto", profile: { id: "u1" },
    }));
  });
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
    assert.ok(variants.some((text) => /synthetic-(offer|unknown)-/.test(text)), "источник отличия должен быть показан");

    const hrefs = await page.locator(".demo-card a.demo-card__link").evaluateAll(
      (links) => links.map((link) => link.getAttribute("href")),
    );
    assert.ok(hrefs.length >= 3, "у каждой карточки должна быть доступная ссылка");
    assert.ok(hrefs.every((href) => href && href.startsWith("http")));
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

  await t.test("HTTP-транспорт через собственный Go API", { skip: !apiUp ? "Go API не запущен" : false }, async () => {
    const transport = page.locator("select").nth(1);
    await profile.selectOption("u1");
    await scenario.selectOption("auto");
    await transport.selectOption("http");
    await page.waitForFunction(() => document.querySelectorAll(".demo-card").length > 0);
    const cards = await page.locator(".demo-card").count();
    assert.ok(cards > 0 && cards <= 3);
    const status = await page.locator(".demo-status").first().textContent();
    assert.match(status, /request_id/);
    if (process.env.RECSYS_E2E_LIVE === "1") {
      await page.getByLabel("Каталог").selectOption("DEMO-BASE");
      await page.getByRole("button", { name: "Купить из каталога" }).click();
      await page.getByText("Покупка записана для DEMO-BASE").waitFor();
      const renewal = page.locator(".demo-card").filter({ has: page.getByText("продление", { exact: true }) }).first();
      await renewal.getByRole("button", { name: "Сравнить варианты" }).click();
      await renewal.getByText("DEMO-RENEW-12 · 12 мес.").waitFor();
      await renewal.getByText("DEMO-RENEW-24 · 24 мес.").waitFor();
      await renewal.getByRole("button", { name: "Почему рекомендуем" }).click();
      assert.match(await renewal.locator(".demo-card__source").first().textContent(), /item:DEMO-BASE → item:DEMO-RENEW-12/);
      await renewal.getByRole("button", { name: "Купить DEMO-RENEW-12" }).click();
      await page.getByText("Покупка записана для DEMO-RENEW-12").waitFor();
      await page.getByRole("heading", { name: /24/ }).waitFor();
      assert.equal(await page.getByText("DEMO-RENEW-12 · 12 мес.").count(), 0);
    }
    await transport.selectOption("mock");
  });

  await t.test("старые экраны открываются в live-режиме", async () => {
    await page.goto(`${ORIGIN}/#/`);
    await page.getByText("Вам может подойти").waitFor();
    assert.ok(await page.locator(".home-page .heading-xl").count() > 0);

    await page.goto(`${ORIGIN}/#/recommendations`);
    await page.getByText("Тест рекомендаций").first().waitFor();
    const stats = await page.locator(".stat-card__value").allTextContents();
    assert.ok(stats.includes("LIVE"));
  });

  await t.test("карточки видны на узком экране", async () => {
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(SCREEN);
    await page.getByText("Открыть карточку").first().waitFor();
    const boxes = await page.locator(".demo-card").first().boundingBox();
    assert.ok(boxes && boxes.width > 0 && boxes.width <= 390);
  });

  await t.test("консоль без ошибок приложения", async () => {
    assert.deepEqual(consoleErrors, []);
  });
});
