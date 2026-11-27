export const ROUTES = [
  "overview",
  "demo",
  "demo-product",
  "recommendations",
  "catalog",
  "invoices",
  "orders",
  "keys",
  "trials",
  "contacts",
  "notifications",
  "profile",
  "subscriptions",
  "shop",
  "favorites",
] as const;

export type RouteKey = (typeof ROUTES)[number];

export const ROUTE_TITLES: Record<RouteKey, { title: string; caption: string }> = {
  overview: { title: "Главная", caption: "Личный кабинет" },
  demo: { title: "Рекомендации", caption: "Демо ЛК: карточки и варианты" },
  "demo-product": { title: "Товар", caption: "Демо ЛК: полная информация о товаре" },
  recommendations: { title: "Тест рекомендаций", caption: "Проверка БД и алгоритма" },
  catalog: { title: "Продукты", caption: "Каталог Softline" },
  invoices: { title: "Счета и платежи", caption: "Счета и платежи" },
  orders: { title: "Мои заказы", caption: "История заказов" },
  keys: { title: "Ключи и дистрибутивы", caption: "Лицензии и ключи" },
  trials: { title: "Пробные лицензии", caption: "Пробные версии" },
  contacts: { title: "Контакты", caption: "КонтактыSoftline" },
  notifications: { title: "Уведомления", caption: "Уведомления" },
  profile: { title: "Профиль", caption: "Настройки интеграции" },
  subscriptions: { title: "Подписки", caption: "Активные подписки" },
  shop: { title: "Интернет‑магазин", caption: "Каталог продуктов" },
  favorites: { title: "Избранное", caption: "Избранные продукты" },
};

export function isRouteKey(value: string): value is RouteKey {
  return (ROUTES as readonly string[]).includes(value);
}

export function parseRoute(hash: string): RouteKey {
  const normalized = hash.replace(/^#\/?/, "").split("?")[0].trim();
  return isRouteKey(normalized) ? normalized : "overview";
}

export function getRouteParam(name: string, hash = window.location.hash): string | null {
  const query = hash.split("?")[1] ?? "";
  return new URLSearchParams(query).get(name);
}
