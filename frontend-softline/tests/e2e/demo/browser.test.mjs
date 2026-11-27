import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { execFileSync } from "node:child_process";
import { chromium } from "playwright-core";

const ORIGIN = (process.env.FRONTEND_URL ?? "http://localhost:5173").replace(/\/$/, "");
const SCREEN = `${ORIGIN}/`;

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
  if (process.env.PREVIEW_BROWSER_RESOLVE) return true;
  try {
    const response = await fetch(ORIGIN, { signal: AbortSignal.timeout(2000) });
    return response.ok;
  } catch {
    return false;
  }
}

const executablePath = findBrowser();
const live = Boolean(process.env.PREVIEW_ACCOUNT_PASSWORD && process.env.PREVIEW_ADMIN_PASSWORD);
const fixture = JSON.parse(readFileSync(new URL("../../../../recsys/api/examples/recommendations.full.json", import.meta.url), "utf8"));
const profiles = {
  notver: { role: "unverified", client_group: "B2C", label: "Неверифицированный аккаунт" },
  verb2b: { role: "verified", client_group: "B2B", label: "Верифицированный B2B" },
  verb2c: { role: "verified", client_group: "B2C", label: "Верифицированный B2C" },
  admin: { role: "admin", client_group: "B2C", label: "Администратор" },
};

test("главная для гостя, три статуса и закрытая диагностика", {
  skip: !await serverUp() || !executablePath ? "Нужны доступный стенд и установленный Chromium" : false,
}, async (t) => {
  const args = ["--no-sandbox"];
  if (process.env.PREVIEW_BROWSER_RESOLVE) {
    args.push("--no-proxy-server", `--host-resolver-rules=${process.env.PREVIEW_BROWSER_RESOLVE}`);
  }
  const browser = await chromium.launch({ executablePath, headless: true, args });
  t.after(() => browser.close());
  const page = await browser.newPage();

  if (!live) {
    await page.route("**/api/v1/**", async (route) => {
      const request = route.request();
      const url = new URL(request.url());
      const path = url.pathname.slice("/api/v1".length);
      const profile = request.headers().authorization?.slice(7);
      if (path === "/auth/login") {
        const { login, password } = request.postDataJSON();
        if (!Object.hasOwn(profiles, login) || password !== "fixture") {
          return route.fulfill({ status: 401, json: { error: { message: "Invalid credentials" } } });
        }
        return route.fulfill({ json: { access_token: login } });
      }
      if (path === "/users/me" && Object.hasOwn(profiles, profile)) {
        return route.fulfill({ json: { ...profiles[profile], profile_id: profile, client_id: profile, login: profile } });
      }
      if (path === "/recommendations/guest" || path === "/recommendations/me") {
        return route.fulfill({ json: fixture });
      }
      if (path === "/products") {
        return route.fulfill({ json: { items: [{ item_id: "DEMO-BASE", title: "Базовая лицензия", available: true }] } });
      }
      if (path === "/admin/recommendations" && profile === "admin") {
        return route.fulfill({ json: {
          target: url.searchParams.get("target"), recommendations: fixture, duration_ms: 12,
          tables: { catalog_offer_embeddings: 8 }, runs: [],
        } });
      }
      return route.fulfill({ status: 403, json: { error: { message: "Administrator access required" } } });
    });
  }

  const guest = page.waitForResponse((r) => r.url().includes("/recommendations/guest") && r.ok());
  await page.goto(SCREEN);
  assert.equal(new URL((await guest).url()).searchParams.get("limit"), "3");
  await page.getByRole("heading", { name: "Популярные товары" }).waitFor();
  await page.locator(".store-front .portal-suggestion").first().waitFor();
  assert.equal(await page.getByRole("link", { name: "Отладка рекомендаций" }).count(), 0);
  assert.equal(await page.getByRole("button", { name: "Купить товар" }).count(), 0);
  await page.waitForFunction(() => [...document.images].filter((image) => image.closest(".store-promos")).every((image) => image.complete && image.naturalWidth > 0));

  const search = page.getByRole("search");
  const term = search.getByRole("textbox");
  const runsWithLimit = (limit) => page.waitForResponse((response) =>
    response.url().includes("/recommendations/guest") &&
    new URL(response.url()).searchParams.get("limit") === limit && response.ok());
  await term.fill("модуль");
  const expanded = runsWithLimit("36");
  await search.getByRole("button", { name: "Поиск" }).click();
  await expanded;
  await term.fill("");
  const compact = runsWithLimit("3");
  await search.getByRole("button", { name: "Поиск" }).click();
  await compact;

  async function enter(name) {
    await page.locator(".store-actions").getByRole("button", { name: "Войти", exact: true }).click();
    const modal = page.getByRole("dialog", { name: "Вход на демостенд" });
    await modal.getByLabel("Логин", { exact: true }).fill(name);
    await modal.getByLabel("Пароль", { exact: true }).fill(live
      ? (name === "admin" ? process.env.PREVIEW_ADMIN_PASSWORD : process.env.PREVIEW_ACCOUNT_PASSWORD)
      : "fixture");
    const signedIn = page.waitForResponse((r) => r.url().includes("/recommendations/me") && r.ok());
    await modal.getByRole("button", { name: "Войти", exact: true }).click();
    assert.equal(new URL((await signedIn).url()).searchParams.get("limit"), "3");
    await page.locator(".sidebar__username").getByText(name, { exact: true }).waitFor();
    await page.locator(".sidebar__email").getByText(profiles[name].label, { exact: true }).waitFor();
    await page.getByRole("heading", { name: "Вам может подойти" }).waitFor();
  }

  for (const name of ["notver", "verb2b", "verb2c"]) {
    await enter(name);
    await page.locator(".portal-overview .portal-suggestion").first().waitFor();
    assert.equal(await page.getByRole("link", { name: "Отладка рекомендаций" }).count(), 0);
    await page.goto(`${ORIGIN}/#/recommendations`);
    await page.getByText("Для просмотра диагностики нужен аккаунт администратора.").waitFor();
    await page.locator(".b-store-header__middle-link.is-active").click();
    await page.getByRole("button", { name: "Выйти", exact: true }).click();
    await page.getByRole("heading", { name: "Популярные товары" }).waitFor();
  }

  await enter("admin");
  await page.locator(".sidebar").getByRole("link", { name: "Отладка рекомендаций" }).click();
  await page.getByRole("heading", { name: "Отладка рекомендаций" }).waitFor();
  await page.getByText("catalog_offer_embeddings", { exact: true }).waitFor();
  const comparison = page.waitForResponse((r) => r.url().includes("/admin/recommendations?target=verb2b") && r.ok());
  await page.getByLabel("Профиль для проверки").selectOption("verb2b");
  await comparison;
  assert.equal(await page.getByLabel("История", { exact: true }).inputValue(), "clean");
  if (live) {
    await page.getByLabel("История", { exact: true }).selectOption("latest");
    await page.getByText(/покупок \d+/).waitFor();
    await page.getByLabel("История", { exact: true }).selectOption("clean");
  }
  await page.getByText("Ответ ранжирования, оценки и граф", { exact: true }).waitFor();
  await page.getByRole("button", { name: "Показать граф" }).first().click();
  await page.getByRole("dialog").waitFor();
  await page.keyboard.press("Escape");

  await page.setViewportSize({ width: 1024, height: 844 });
  await page.locator(".b-store-header__menu-btn").click();
  await page.locator(".portal-sidebar-drawer").getByRole("link", { name: "Отладка рекомендаций" }).waitFor();
  await page.locator(".b-store-header__menu-btn").click();

  await page.setViewportSize({ width: 390, height: 844 });
  const mobile = page.locator(".portal-account-mobile");
  await mobile.getByRole("link", { name: "Отладка рекомендаций" }).waitFor();
  await mobile.getByRole("link", { name: /admin/ }).click();
  await page.getByRole("button", { name: "Выйти", exact: true }).click();
  const storeNav = page.locator(".store-mobile-nav");
  await storeNav.getByRole("button", { name: "Войти" }).click();
  await page.getByRole("dialog", { name: "Вход на демостенд" }).waitFor();
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), 390);
});
