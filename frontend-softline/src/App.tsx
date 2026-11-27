import { useEffect, useState } from "react";
import { AppShell } from "./components/AppShell";
import { parseRoute, type RouteKey } from "./lib/routes";
import { CatalogPage } from "./pages/CatalogPage";
import { DiagnosticsPage } from "./pages/DemoPage";
import { DemoProductPage } from "./pages/DemoProductPage";
import { OverviewPage } from "./pages/OverviewPage";
import { StorefrontPage } from "./pages/StorefrontPage";
import { InvoicesPage, KeysPage, NotificationsPage, OrdersPage, StaticPage } from "./pages/TablesPage";
import { DebugProvider } from "./state/debug";
import { profileStatus, SessionProvider, useSession } from "./state/session";
import { SettingsProvider } from "./state/settings";

function ProfilePage() {
  const { profile, logout } = useSession();
  if (!profile) return null;
  return (
    <div className="react-page-inner">
      <h1>Мой профиль</h1>
      <p>{profile.login} · {profileStatus(profile)}</p>
      <button className="clone-link-btn" type="button" onClick={() => { logout(); window.location.hash = "#/overview"; }}>Выйти</button>
    </div>
  );
}

function Page({ route }: { route: RouteKey }) {
  switch (route) {
    case "overview":
    case "demo":
      return <OverviewPage />;
    case "demo-product":
      return <DemoProductPage />;
    case "recommendations":
      return <DiagnosticsPage />;
    case "catalog":
    case "shop":
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
    case "favorites":
      return <StaticPage title="Избранное" text="Сохранённые продукты." />;
  }
}

function Portal({ route }: { route: RouteKey }) {
  const { profile } = useSession();
  useEffect(() => {
    document.title = profile ? "Личный кабинет · Softline" : "Интернет-магазин Softline · Демостенд";
  }, [profile]);
  if (!profile) return <StorefrontPage openLogin={route === "profile"} restricted={route === "recommendations"} />;
  return (
    <AppShell key={profile.profile_id} profile={profile} route={route}>
      <Page route={route} />
    </AppShell>
  );
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
          <Portal route={route} />
        </DebugProvider>
      </SessionProvider>
    </SettingsProvider>
  );
}
