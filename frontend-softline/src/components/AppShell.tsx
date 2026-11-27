import { useEffect, useMemo, useRef, type FormEvent, type MouseEvent, type ReactNode } from "react";
import { HEADER_HTML, SIDEBAR_HTML } from "../data/chrome";
import { useCloneUi } from "../hooks/useCloneUi";
import type { RouteKey } from "../lib/routes";
import { profileStatus, useSession, type SessionProfile } from "../state/session";
import { Fragment } from "./Fragment";

function portalChrome(html: string, profile: SessionProfile) {
  const document = new DOMParser().parseFromString(html, "text/html");
  const label = profileStatus(profile);
  const values = {
    "DEMO_LOGIN_LABEL": profile.login,
    "DEMO_STATUS_LABEL": label,
    "DEMO_INITIALS": profile.login.slice(0, 2).toUpperCase(),
  };
  const text = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  while (text.nextNode()) {
    const node = text.currentNode;
    for (const [slot, value] of Object.entries(values)) {
      if (node.textContent?.trim() === slot) node.textContent = value;
    }
  }

  const nav = document.querySelector(".sidebar__window_main .nav-list");
  const adminItem = nav?.querySelector('a[href="#/recommendations"]')?.closest("li");
  const logout = document.querySelector<HTMLButtonElement>(".b-store-header__dropdown-header")?.closest("ul")
    ?.querySelector<HTMLButtonElement>("button");
  if (logout) logout.dataset.portalLogout = "";
  if (profile.role === "admin" && adminItem) {
    adminItem.querySelector(".nav-list__text")!.textContent = "Отладка рекомендаций";
  } else {
    adminItem?.remove();
  }
  for (const path of ["#/demo", "#/demo-product"]) nav?.querySelector(`a[href="${path}"]`)?.closest("li")?.remove();
  document.querySelectorAll<HTMLAnchorElement>('a[href^="#/overview"]').forEach((link) => {
    const href = link.getAttribute("href") ?? "";
    const target = href.slice("#/overview".length);
    if (target === "catalog/") link.href = "#/catalog";
    else if (target === "favorites/") link.href = "#/favorites";
    else if (target === "basket/") link.href = "#/orders";
    else if (target) {
      link.href = "https://store.softline.ru/" + target;
      link.target = "_blank";
      link.rel = "noreferrer";
    }
  });
  return document.body.innerHTML;
}

interface AppShellProps {
  route: RouteKey;
  profile: SessionProfile;
  children: ReactNode;
}

export function AppShell({ route, profile, children }: AppShellProps) {
  const { logout } = useSession();
  const rootRef = useRef<HTMLDivElement>(null);
  const header = useMemo(() => portalChrome(HEADER_HTML, profile), [profile]);
  const sidebar = useMemo(() => portalChrome(SIDEBAR_HTML, profile), [profile]);
  useCloneUi(rootRef, route);

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const path = route === "demo" ? "overview" : route;
    root.querySelectorAll<HTMLElement>(".nav-list__link, .b-store-header__bottom-short-link").forEach((link) => {
      const active = link.getAttribute("href") === "#/" + path;
      link.classList.toggle("is-active", active);
      link.classList.toggle("router-link-active", active);
      link.classList.toggle("router-link-exact-active", active);
      if (active) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    });
  }, [route]);

  function onClick(event: MouseEvent<HTMLDivElement>) {
    const target = event.target;
    if (!(target instanceof Element) || !target.closest("[data-portal-logout]")) return;
    event.preventDefault();
    logout();
    window.location.hash = "#/overview";
  }

  function onSubmit(event: FormEvent<HTMLDivElement>) {
    const form = event.target;
    if (!(form instanceof HTMLFormElement) || !form.matches(".b-search-input__box")) return;
    event.preventDefault();
    const query = form.querySelector("input")?.value.trim() ?? "";
    window.location.hash = "#/catalog" + (query ? "?q=" + encodeURIComponent(query) : "");
  }

  return (
    <div className="app__content react-page portal" ref={rootRef} onClick={onClick} onSubmit={onSubmit}>
      <div className="layout-with-sidebar">
        <Fragment html={header} />
        <div className="portal-account-mobile">
          <a href="#/profile">{profile.login} · {profileStatus(profile)}</a>
          {profile.role === "admin" && <a href="#/recommendations">Отладка рекомендаций</a>}
        </div>
        <div className="layout main-container">
          <div className="mobile-hidden react-sidebar-slot">
            <Fragment html={sidebar} />
          </div>
          <main className="layout-with-sidebar-container react-content">
            <div className="layout-with-sidebar-content layout-with-sidebar-container__content">
              {children}
            </div>
          </main>
        </div>
      </div>
    </div>
  );
}
