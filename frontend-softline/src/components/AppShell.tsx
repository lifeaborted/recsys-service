import { useEffect, useRef, type ReactNode } from "react";
import { HEADER_HTML, SIDEBAR_HTML } from "../data/chrome";
import { useCloneUi } from "../hooks/useCloneUi";
import type { RouteKey } from "../lib/routes";
import { DebugPanel } from "./DebugPanel";
import { Fragment } from "./Fragment";

const HASH: Record<RouteKey, string> = {
  overview: "#/overview",
  demo: "#/demo",
  recommendations: "#/recommendations",
  catalog: "#/catalog",
  invoices: "#/invoices",
  orders: "#/orders",
  keys: "#/keys",
  trials: "#/trials",
  contacts: "#/contacts",
  notifications: "#/notifications",
  profile: "#/profile",
  subscriptions: "#/subscriptions",
  shop: "#/shop",
  favorites: "#/favorites",
};

interface AppShellProps {
  route: RouteKey;
  children: ReactNode;
}

export function AppShell({ route, children }: AppShellProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  useCloneUi(rootRef, route);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    root.querySelectorAll<HTMLElement>(".nav-list__link, .b-store-header__bottom-short-link").forEach((link) => {
      const active = link.getAttribute("href") === HASH[route];
      link.classList.toggle("is-active", active);
      link.classList.toggle("router-link-active", active);
      link.classList.toggle("router-link-exact-active", active);
      if (active) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    });
  }, [route]);

  return (
    <div className="app__content react-page" ref={rootRef}>
      <div className="layout-with-sidebar">
        <Fragment html={HEADER_HTML} />
        <div className="layout main-container">
          <div className="mobile-hidden react-sidebar-slot">
            <Fragment html={SIDEBAR_HTML} />
          </div>
          <main className="layout-with-sidebar-container react-content">
            <div className="layout-with-sidebar-content layout-with-sidebar-container__content">
              {children}
            </div>
          </main>
        </div>
      </div>
      <DebugPanel />
    </div>
  );
}
