import { useEffect, useState } from "react";
import { fetchProductsPage, RecsysError, type ProductPage } from "../api/client";
import { RecommendationCard } from "../components/RecommendationCard";
import { useDebug } from "../state/debug";
import { getRouteParam } from "../lib/routes";
import { RECSYS_API, useSession } from "../state/session";

const PAGE_SIZE = 48;

export function CatalogPage() {
  const session = useSession();
  const { showScores } = useDebug();
  const [page, setPage] = useState(0);
  const [query, setQuery] = useState(() => getRouteParam("q") ?? "");
  const [catalog, setCatalog] = useState<ProductPage | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const token = session.profile?.token ?? "";
  useEffect(() => {
    function updateQuery() {
      setQuery(getRouteParam("q") ?? "");
      setPage(0);
    }
    window.addEventListener("hashchange", updateQuery);
    return () => window.removeEventListener("hashchange", updateQuery);
  }, []);


  useEffect(() => {
    if (!token) {
      setError("Нет активной сессии. Войдите в демо-профиль.");
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setError(null);
    fetchProductsPage(
      { baseUrl: RECSYS_API, token, timeoutMs: 5000 },
      { limit: PAGE_SIZE, offset: page * PAGE_SIZE, q: query },
    ).then((result) => {
      if (controller.signal.aborted) return;
      setCatalog(result);
      setLoading(false);
    }).catch((cause: unknown) => {
      if (controller.signal.aborted) return;
      setError(cause instanceof RecsysError || cause instanceof Error ? cause.message : "Не удалось загрузить каталог");
      setLoading(false);
    });
    return () => controller.abort();
  }, [token, page, query]);

  const total = catalog?.total ?? 0;
  const from = total ? page * PAGE_SIZE + 1 : 0;
  const to = Math.min((page + 1) * PAGE_SIZE, total);

  return (
    <div className="react-page-inner">
      <h1>Продукты</h1>
      <p className="react-page__lead">
        Карточки товаров из текущего фида PostgreSQL. Поиск и постраничный просмотр.
      </p>

      <div className="clone-toolbar">
        <input
          value={query}
          onChange={(event) => {
            setQuery(event.target.value);
            setPage(0);
          }}
          placeholder="Название или артикул"
        />
        <span className="reco-chip">Найдено: {total}</span>
      </div>

      {loading ? <div className="demo-status">Загружаем каталог…</div> : null}
      {error ? <div className="demo-banner demo-banner_error">{error}</div> : null}

      <div className="clone-product-grid">
        {(catalog?.items ?? []).map((item) => (
          <RecommendationCard key={item.item_id} item={item} showScores={showScores} />
        ))}
        {!loading && !(catalog?.items.length) ? <div className="reco-empty">Товары не найдены</div> : null}
      </div>

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
  );
}
