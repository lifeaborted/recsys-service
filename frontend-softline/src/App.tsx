import { useEffect, useState } from "react";
import { AppShell } from "./components/AppShell";
import { parseRoute, type RouteKey } from "./lib/routes";
import { CatalogPage } from "./pages/CatalogPage";
import { DemoPage } from "./pages/DemoPage";
import { DemoProductPage } from "./pages/DemoProductPage";
import { InvoicesPage, KeysPage, NotificationsPage, OrdersPage, StaticPage } from "./pages/TablesPage";
import { OverviewPage } from "./pages/OverviewPage";
import { ProfilePage } from "./pages/ProfilePage";
import { RecommendationsPage } from "./pages/RecommendationsPage";
import { DebugProvider } from "./state/debug";
import { SessionProvider } from "./state/session";
import { SettingsProvider } from "./state/settings";

function Page({ route }: { route: RouteKey }) {
  switch (route) {
    case "overview":
      return <OverviewPage />;
    case "demo":
      return <DemoPage />;
    case "demo-product":
      return <DemoProductPage />;
    case "recommendations":
      return <RecommendationsPage />;
    case "catalog":
      return <CatalogPage />;
    case "invoices":
      return <InvoicesPage />;
    case "orders":
      return <OrdersPage />;
    case "keys":
      return <KeysPage />;
    case "notifications":
      return <NotificationsPage />;
    case "profile":
      return <ProfilePage />;
    case "trials":
      return <StaticPage title="Пробные лицензии" text="Пробные версии продуктов доступны по запросу." />;
    case "contacts":
      return <StaticPage title="Контакты" text="sales@softline.com · 8 800 232-00-23" />;
    case "subscriptions":
      return <StaticPage title="Подписки" text="Активные подписки вашей компании." />;
    case "shop":
      return <StaticPage title="Интернет‑магазин" text="Каталог продуктов Softline." />;
    case "favorites":
      return <StaticPage title="Избранное" text="Сохранённые продукты." />;
    default:
      return <OverviewPage />;
  }
}

export function App() {
  const [route, setRoute] = useState<RouteKey>(() => parseRoute(window.location.hash));

  useEffect(() => {
    const handleHashChange = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener("hashchange", handleHashChange);
    return () => window.removeEventListener("hashchange", handleHashChange);
  }, []);

  useEffect(() => {
    window.scrollTo({ top: 0 });
  }, [route]);

  return (
    <SettingsProvider>
      <SessionProvider>
        <DebugProvider>
          <AppShell route={route}>
            <Page route={route} />
          </AppShell>
        </DebugProvider>
      </SessionProvider>
    </SettingsProvider>
  );
}
