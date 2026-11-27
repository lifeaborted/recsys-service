import { useEffect, useState } from "react";
import {
  fetchProductDetail,
  fetchProductsPage,
  fetchScoreHistory,
  RecsysError,
  type ProductPage,
} from "../api/client";
import type { ProductDetail, ScoreHistory } from "../api/types";
import { DemoScoreModal } from "../components/DemoScoreModal";
import { formatPrice } from "../lib/format";
import { getRouteParam } from "../lib/routes";
import { useSession } from "../state/session";
import { useSettings } from "../state/settings";

function textValue(value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

const PAGE_SIZE = 20;

export function DemoProductPage() {
  const { settings } = useSettings();
  const session = useSession();
  const [itemID, setItemID] = useState(() => getRouteParam("item") ?? "");
  const [product, setProduct] = useState<ProductDetail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [nonce, setNonce] = useState(0);

  const [page, setPage] = useState(0);
  const [query, setQuery] = useState("");
  const [withHistory, setWithHistory] = useState(false);
  const [list, setList] = useState<ProductPage | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [listLoading, setListLoading] = useState(false);
  const [scoreHistory, setScoreHistory] = useState<ScoreHistory | null>(null);
  const [scoreError, setScoreError] = useState<string | null>(null);
  const [scoreLoading, setScoreLoading] = useState(false);

  const token = settings.token || session.profile?.token || "";
  const baseUrl = settings.baseUrl || "/api";

  const openScore = (target: string) => {
    setScoreHistory(null);
    setScoreError(null);
    setScoreLoading(true);
    fetchScoreHistory({ baseUrl, token, timeoutMs: settings.timeoutMs }, target)
      .then((result) => {
        setScoreHistory(result);
        setScoreLoading(false);
      })
      .catch((cause: unknown) => {
        setScoreError(cause instanceof RecsysError || cause instanceof Error ? cause.message : "Не удалось загрузить график");
        setScoreLoading(false);
      });
  };

  useEffect(() => {
    const sync = () => {
      setItemID(getRouteParam("item") ?? "");
      setPage(0);
    };
    window.addEventListener("hashchange", sync);
    return () => window.removeEventListener("hashchange", sync);
  }, []);

  useEffect(() => {
    if (itemID) return;
    if (!token) {
      setListError(session.transport === "mock" ? "Нет активной сессии. Войдите на демо-экран." : null);
      setListLoading(session.transport !== "mock");
      return;
    }
    const controller = new AbortController();
    setListLoading(true);
    setListError(null);
    fetchProductsPage(
      { baseUrl, token, timeoutMs: settings.timeoutMs },
      { limit: PAGE_SIZE, offset: page * PAGE_SIZE, q: query, withHistory },
    ).then((result) => {
      if (controller.signal.aborted) return;
      setList(result);
      setListLoading(false);
    }).catch((cause: unknown) => {
      if (controller.signal.aborted) return;
      setListError(cause instanceof RecsysError || cause instanceof Error ? cause.message : "Не удалось загрузить каталог");
      setListLoading(false);
    });
    return () => controller.abort();
  }, [itemID, token, baseUrl, settings.timeoutMs, page, query, withHistory, session.transport]);

  useEffect(() => {
    if (!itemID || !token) return;
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    fetchProductDetail(
      { baseUrl, token, timeoutMs: settings.timeoutMs },
      itemID,
    ).then((result) => {
      if (controller.signal.aborted) return;
      setProduct(result);
      setLoading(false);
    }).catch((cause: unknown) => {
      if (controller.signal.aborted) return;
      setError(cause instanceof RecsysError || cause instanceof Error ? cause.message : "Не удалось загрузить товар");
      setLoading(false);
    });
    return () => controller.abort();
  }, [itemID, token, baseUrl, settings.timeoutMs, nonce]);

  if (!itemID) {
    const total = list?.total ?? 0;
    const from = total ? page * PAGE_SIZE + 1 : 0;
    const to = Math.min((page + 1) * PAGE_SIZE, total);
    return (
      <div className="react-page-inner">
        <div className="clone-card demo-product__section">
          <div className="demo-product__section-head">
            <div>
              <h3>Все товары из БД</h3>
              <p className="demo-product__notes">Найдено: {total}</p>
            </div>
            <div className="demo-product__search">
              <input
                type="search"
                value={query}
                placeholder="Название или артикул"
                aria-label="Поиск товаров"
                onChange={(event) => {
                  setQuery(event.target.value);
                  setPage(0);
                }}
              />
              <button className="clone-link-btn" type="button" onClick={() => { setQuery(""); setPage(0); }}>Сбросить</button>
              <label className="reco-limit">
                <input
                  type="checkbox"
                  checked={withHistory}
                  onChange={(event) => {
                    setWithHistory(event.target.checked);
                    setPage(0);
                  }}
                />
                Только с историей оценок
              </label>
            </div>
          </div>

          {listLoading ? <div className="demo-status">Загружаем каталог…</div> : null}
          {listError ? <div className="demo-banner demo-banner_error">{listError}</div> : null}

          <table className="clone-table demo-product__list">
            <thead>
              <tr>
                <th>item_id</th>
                <th>Товар</th>
                <th>Категория</th>
                <th>Производитель</th>
                <th style={{ textAlign: "right" }}>Цена</th>
                <th>Наличие</th>
                <th>График</th>
              </tr>
            </thead>
            <tbody>
              {(list?.items ?? []).map((item) => (
                <tr key={item.item_id}>
                  <td><a href={`#/demo-product?item=${encodeURIComponent(item.item_id)}`}>{item.item_id}</a></td>
                  <td>{item.title}</td>
                  <td>{item.category || "—"}</td>
                  <td>{item.subtitle || "—"}</td>
                  <td style={{ textAlign: "right" }}>{formatPrice(item.price, item.currency)}</td>
                  <td>{item.offer_id ? "в каталоге" : "—"}</td>
                  <td>
                    <button className="clone-link-btn" type="button" onClick={() => openScore(item.item_id)}>
                      График score
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {!listLoading && !(list?.items.length) ? <div className="reco-empty">Товары не найдены.</div> : null}

          <div className="demo-product__pager">
            <span>{from}–{to} из {total}</span>
            <button className="clone-link-btn" type="button" disabled={page === 0} onClick={() => setPage((value) => value - 1)}>← Назад</button>
            <button
              className="clone-link-btn"
              type="button"
              disabled={to >= total}
              onClick={() => setPage((value) => value + 1)}
            >
              Вперёд →
            </button>
          </div>
        </div>
        {scoreLoading ? <div className="demo-status">Загружаем график…</div> : null}
        {scoreError ? <div className="demo-banner demo-banner_error">{scoreError}</div> : null}
        {scoreHistory ? (
          <DemoScoreModal history={scoreHistory} onClose={() => setScoreHistory(null)} />
        ) : null}
      </div>
    );
  }

  return (
    <div className="react-page-inner">
      <div className="demo-product__nav">
        <a className="clone-link-btn" href="#/demo-product">← Все товары</a>
        <a className="clone-link-btn" href="#/demo">← Назад к демо</a>
        <button className="clone-link-btn" type="button" onClick={() => openScore(itemID)}>График score</button>
        <span className="demo-product__id">{itemID}</span>
      </div>

      {loading ? <div className="demo-status">Загружаем данные товара…</div> : null}
      {error ? <div className="demo-banner demo-banner_error">{error}</div> : null}

      {product && !loading ? (
        <>
          <div className="clone-card demo-product__hero">
            <div className="demo-product__image">
              {product.picture_url ? (
                <img src={product.picture_url} alt={product.title} />
              ) : (
                <span className="demo-product__image-empty">Нет изображения</span>
              )}
            </div>
            <div className="demo-product__summary">
              <div className="demo-card__badges">
                <span className="demo-card__kind">{product.category || "без категории"}</span>
                <span className={product.available ? "demo-card__verification demo-card__verification_confirmed" : "demo-card__verification demo-card__verification_check"}>
                  {product.available ? "доступен" : "нет в наличии"}
                </span>
              </div>
              <h2 className="demo-product__title">{product.title}</h2>
              <p className="demo-product__vendor">{product.vendor || "Производитель не указан"}</p>
              <div className="demo-product__prices">
                <strong>{formatPrice(product.price, product.currency_code || "RUB")}</strong>
                {product.old_price ? <span>{formatPrice(product.old_price, product.currency_code || "RUB")}</span> : null}
              </div>
              <div className="demo-product__links">
                {product.url ? (
                  <a className="button button_primary button_size-l" href={product.url} target="_blank" rel="noreferrer">Открыть на сайте</a>
                ) : null}
                {product.vendor_url ? (
                  <a className="button button_default button_size-l" href={product.vendor_url} target="_blank" rel="noreferrer">Сайт производителя</a>
                ) : null}
              </div>
              {product.sales_notes ? <p className="demo-product__notes">{product.sales_notes}</p> : null}
            </div>
          </div>

          <div className="clone-card demo-product__section">
            <h3>Основная информация</h3>
            <div className="stat-grid demo-product__facts">
              {[
                ["item_id", product.item_id],
                ["offer_id", product.offer_id],
                ["SKU", product.sku],
                ["Категория", product.category],
                ["Производитель", product.vendor],
                ["Доставка", product.delivery ? "да" : "нет"],
                ["Самовывоз", product.pickup ? "да" : "нет"],
                ["Наличие", product.available ? "есть" : "нет"],
              ].map(([label, value]) => (
                <div className="stat-card" key={label}>
                  <div className="stat-card__value" title={textValue(value)}>{textValue(value)}</div>
                  <div className="stat-card__label">{label}</div>
                </div>
              ))}
            </div>
          </div>

          <div className="clone-card demo-product__section">
            <h3>Описание</h3>
            <p className="demo-product__description">{product.description || "Описание в фиде отсутствует."}</p>
          </div>

          <div className="clone-card demo-product__section">
            <div className="demo-product__section-head">
              <h3>Характеристики</h3>
              <button className="clone-link-btn" type="button" onClick={() => setNonce((value) => value + 1)}>Обновить из БД</button>
            </div>
            {product.parameters.length ? (
              <table className="clone-table">
                <tbody>
                  {product.parameters.map((param, index) => (
                    <tr key={`${param.name}-${index}`}>
                      <th scope="row">{param.name}</th>
                      <td>{textValue(param.value)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div className="reco-empty">Характеристик в фиде нет.</div>
            )}
          </div>
        </>
      ) : null}
      {scoreLoading ? <div className="demo-status">Загружаем график…</div> : null}
      {scoreError ? <div className="demo-banner demo-banner_error">{scoreError}</div> : null}
      {scoreHistory ? (
        <DemoScoreModal history={scoreHistory} onClose={() => setScoreHistory(null)} />
      ) : null}
    </div>
  );
}
