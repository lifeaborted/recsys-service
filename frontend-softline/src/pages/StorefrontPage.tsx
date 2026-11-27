import { useEffect, useRef, useState, type FormEvent } from "react";
import { PortalRecommendationShelf } from "../components/PortalRecommendationShelf";
import { CATALOG, type CatalogItem } from "../data/catalog";
import { useDemoRecommendations } from "../hooks/useDemoRecommendations";
import { useSession } from "../state/session";
import "./storefront.css";

const assets = "/assets/store";
const store = "https://store.softline.ru";
const promos = [
  { file: "vk-workspace.avif", title: "VK WorkSpace", path: "/vkworkspace/" },
  { file: "remote-desktop.avif", title: "Удалённый рабочий стол с GPT", path: "/advion/udalennyi-rabocii-stol-s-gpt/" },
  { file: "astra-linux.avif", title: "Astra Linux", path: "/specials/detail/astra-linux-dlya-kompanij-do-500-sotrudnikov/" },
  { file: "backup.avif", title: "Резервное копирование", path: "/softline/rezervnoe-kopirovanie/" },
];
const categories = [
  ["Акции", "/specials/"], ["Производители", "/vendors/"], ["Офисные программы", "/ofisnie-programmi/"],
  ["Антивирусы для организаций", "/antivirusi-bezopasnost/"], ["Защита информации", "/antivirusi-bezopasnost/bezopasnost-zaschita-informatsii/"],
  ["Компьютеры", "/computers/"], ["Инферит", "/inferit/"], ["Блог", "/blog/"], ["ИИ", "/delovie-programmi/iskusstvennyi-intellekt/"],
];
const infoLinks = [
  ["Запрос цены", "/writeorder/"], ["Тендеры", "/tenders/"], ["Для юр.лиц", "/b2b/"],
  ["Оплата и доставка", "/delivery_info/"], ["Контакты", "/contacts/"],
];

function StoreIcon({ name }: { name: string }) {
  return <svg className="store-icon" aria-hidden="true"><use href={`${assets}/icons.svg#${name}`} /></svg>;
}

function Links({ items }: { items: string[][] }) {
  return <>{items.map(([label, path]) => <a key={path} href={store + path} target="_blank" rel="noreferrer">
    {label === "Акции" && <StoreIcon name="fire" />}{label}
  </a>)}</>;
}

function CatalogShelf({ title, items }: { title: string; items: CatalogItem[] }) {
  return (
    <section className="store-container store-suggestions" aria-label={title}>
      <h2>{title}</h2>
      <div className="portal-shelf">
        {items.map((item) => (
          <article className="portal-suggestion store-product" key={item.sku}>
            <a className="store-product__image" href={item.url} target="_blank" rel="noreferrer" tabIndex={-1} aria-hidden="true">
              <img src={item.image} alt="" loading="lazy" />
            </a>
            <span className="portal-suggestion__sku">{item.subtitle}</span>
            <h3><a href={item.url} target="_blank" rel="noreferrer">{item.title}</a></h3>
          </article>
        ))}
      </div>
    </section>
  );
}

function LoginDialog({ close }: { close: () => void }) {
  const { signIn, error, loading } = useSession();
  const dialog = useRef<HTMLDialogElement>(null);
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");

  useEffect(() => {
    const modal = dialog.current;
    modal?.showModal();
    return () => modal?.close();
  }, []);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const secret = password;
    setPassword("");
    void signIn(name, secret);
  }

  return (
    <dialog className="store-login" ref={dialog} onCancel={close} aria-labelledby="store-login-title">
      <button className="store-login__close" type="button" aria-label="Закрыть" onClick={close}><StoreIcon name="x-close" /></button>
      <h2 id="store-login-title">Вход на демостенд</h2>
      <p>Используйте выданный тестовый аккаунт.</p>
      <form onSubmit={submit} aria-busy={loading}>
        <label>Логин<input autoFocus value={name} onChange={(event) => setName(event.target.value)} autoComplete="username" required /></label>
        <label>Пароль<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" required /></label>
        {error && <p role="alert">{error}</p>}
        <button className="store-primary" type="submit" disabled={loading}>{loading ? "Выполняем вход…" : "Войти"}</button>
      </form>
    </dialog>
  );
}

export function StorefrontPage({ openLogin, restricted }: { openLogin: boolean; restricted: boolean }) {
  const result = useDemoRecommendations();
  const [showLogin, setShowLogin] = useState(false);
  const [showCatalog, setShowCatalog] = useState(false);
  const [banner, setBanner] = useState(0);
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const shelf = useRef<HTMLElement>(null);

  function closeLogin() {
    setShowLogin(false);
    if (openLogin) window.location.hash = "#/overview";
  }

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setQuery(search.trim().toLocaleLowerCase("ru"));
    shelf.current?.scrollIntoView({ block: "start", behavior: "smooth" });
  }

  const matches = result.data?.items.filter((item) =>
    !query || `${item.title} ${item.sku ?? ""}`.toLocaleLowerCase("ru").includes(query));
  const visiblePromos = [...promos.slice(banner), ...promos.slice(0, banner)];
  const open = () => setShowLogin(true);

  return (
    <div className="store-front">
      <header className="store-header store-container">
        <div className="store-header__top">
          <a href="#/overview" aria-label="Softline"><img src={assets + "/logo.svg"} width="188" height="32" alt="Softline" /></a>
          <span className="store-city"><StoreIcon name="map-marker" />Челябинск</span>
          <nav aria-label="О магазине" className="store-header__info">
            <a className="store-external" href="https://softline.ru/" target="_blank" rel="noreferrer">Softline.ru</a>
            <Links items={infoLinks} />
          </nav>
        </div>
        <div className="store-header__middle">
          <a href="#/overview" className="store-logo-mobile" aria-label="Softline"><img src={assets + "/logo-mobile.svg"} width="22" height="22" alt="" /></a>
          <button type="button" className="store-primary store-catalog" onClick={() => setShowCatalog(!showCatalog)} aria-expanded={showCatalog} aria-controls="store-categories">
            <StoreIcon name="menu" /><span>Каталог</span>
          </button>
          <form className="store-search" onSubmit={submitSearch} role="search">
            <input aria-label="Продукт, производитель или артикул" placeholder="Продукт, производитель или артикул (SKU)" value={search} onChange={(event) => setSearch(event.target.value)} />
            <button type="submit" aria-label="Поиск"><StoreIcon name="store-search" /></button>
          </form>
          <div className="store-contact"><a href="tel:88002000860">8 (800) 200-08-60</a><a href="mailto:sales.r@softline.com">sales.r@softline.com</a></div>
          <nav aria-label="Личный кабинет магазина" className="store-actions">
            <button type="button" onClick={open}><StoreIcon name="chart" />Сравнение</button>
            <button type="button" onClick={open}><StoreIcon name="store-user" />Войти</button>
            <button type="button" onClick={open}><StoreIcon name="heart" />Избранное</button>
            <button type="button" onClick={open}><StoreIcon name="store-cart" />Корзина</button>
          </nav>
        </div>
        <nav className="store-header__bottom" aria-label="Разделы каталога"><Links items={categories} /></nav>
        <nav className="store-header__quick" aria-label="Контакты и ссылки">
          <a href="tel:88002000860">8 (800) 200-08-60</a><Links items={[infoLinks[0], categories[6], categories[7]]} />
        </nav>
        <details className="store-header__service-menu">
          <summary>Контакты и сервисы</summary>
          <nav aria-label="Контакты и сервисы">
            <a href="tel:88002000860">8 (800) 200-08-60</a>
            <a href="mailto:sales.r@softline.com">sales.r@softline.com</a>
            <Links items={infoLinks} />
          </nav>
        </details>
        {showCatalog && <nav id="store-categories" className="store-catalog-menu" aria-label="Каталог"><Links items={categories} /></nav>}
      </header>
      <main>
        {restricted && <p className="store-container" role="status">Для просмотра диагностики нужен аккаунт администратора. <button type="button" className="store-text-button" onClick={open}>Войти на демостенд</button></p>}
        <section className="store-promos" aria-label="Акции магазина">
          <div className="store-promos__track">
            {visiblePromos.map((promo) => <a key={promo.file} href={store + promo.path} target="_blank" rel="noreferrer">
              <img src={assets + "/" + promo.file} alt={promo.title} />
              <small>Реклама</small>
            </a>)}
          </div>
          <button type="button" className="store-prev" aria-label="Предыдущая акция" onClick={() => setBanner((banner + promos.length - 1) % promos.length)}><StoreIcon name="chevron-left" /></button>
          <button type="button" className="store-next" aria-label="Следующая акция" onClick={() => setBanner((banner + 1) % promos.length)}><StoreIcon name="chevron-left" /></button>
        </section>
        <section className="store-container store-suggestions" ref={shelf} aria-labelledby="store-popular-title">
          <h1 id="store-popular-title">Популярные товары</h1>
          {query && <button className="store-text-button" type="button" onClick={() => { setSearch(""); setQuery(""); }}>Сбросить поиск</button>}
          <PortalRecommendationShelf loading={result.loading} error={result.error} items={matches}
            onFavorite={open} empty={query ? "По вашему запросу товары не найдены." : undefined} />
        </section>
        <CatalogShelf title="Офисные программы" items={CATALOG.filter((item) => item.category === "Офисное ПО")} />
        <CatalogShelf title="«Лаборатория Касперского»" items={CATALOG.filter((item) => item.vendor === "«Лаборатория Касперского»")} />
        <section className="store-container store-about">
          <h2>О магазине Softline</h2>
          <nav aria-label="Информация о магазине"><Links items={infoLinks} /></nav>
        </section>
      </main>
      <nav className="store-mobile-nav" aria-label="Меню магазина">
        <a href="#/overview" aria-current="page"><StoreIcon name="home" />Главная</a>
        <button type="button" onClick={() => setShowCatalog(!showCatalog)} aria-expanded={showCatalog}><StoreIcon name="apps" />Каталог</button>
        <button type="button" onClick={open}><StoreIcon name="store-cart" />Корзина</button>
        <a href={store + "/specials/"} target="_blank" rel="noreferrer"><StoreIcon name="fire" />Акции</a>
        <button type="button" onClick={open}><StoreIcon name="store-user" />Войти</button>
      </nav>
      {(showLogin || openLogin) && <LoginDialog close={closeLogin} />}
    </div>
  );
}
