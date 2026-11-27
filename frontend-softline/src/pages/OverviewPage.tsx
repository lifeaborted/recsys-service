import { useEffect, useState } from "react";
import { listProducts, purchase, type CatalogProduct } from "../api/recsysClient";
import { Fragment } from "../components/Fragment";
import { PortalRecommendationShelf } from "../components/PortalRecommendationShelf";
import { EVENTS_HTML, INFO_WIDGETS_HTML, POPULAR_HTML, PROMO_HTML } from "../data/chrome";
import { useDemoRecommendations } from "../hooks/useDemoRecommendations";
import { RECSYS_API, useSession } from "../state/session";

export function OverviewPage() {
  const result = useDemoRecommendations();
  const { profile } = useSession();
  const token = profile?.token ?? "";
  const [products, setProducts] = useState<CatalogProduct[]>([]);
  const [selected, setSelected] = useState("");
  const [notice, setNotice] = useState<{ text: string; failed: boolean } | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    void listProducts({ baseUrl: RECSYS_API, transport: "http", token }, controller.signal)
      .then((catalog) => {
        if (!controller.signal.aborted) setProducts(catalog.items.filter((item) => item.available));
      })
      .catch((error: unknown) => {
        if (!controller.signal.aborted) setNotice({ text: error instanceof Error ? error.message : "Каталог недоступен", failed: true });
      });
    return () => controller.abort();
  }, [token]);

  async function buy(item: string) {
    setBusy(true);
    setNotice(null);
    try {
      await purchase({ baseUrl: RECSYS_API, transport: "http", token }, item);
      setNotice({ text: "Тестовая покупка сохранена. Рекомендации обновляются.", failed: false });
      result.refresh();
    } catch (error) {
      setNotice({ text: error instanceof Error ? error.message : "Покупку не удалось записать", failed: true });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="home-page">
      <Fragment html={INFO_WIDGETS_HTML} className="mobile-hidden" />
      <Fragment html={PROMO_HTML} />

      <section className="flex-column gap-16 portal-overview" aria-labelledby="portal-recommendations">
        <h1 className="heading-xl sl-color-text" id="portal-recommendations">Вам может подойти</h1>
        <PortalRecommendationShelf loading={result.loading} error={result.error} items={result.data?.items} busy={busy}
          onPurchase={(item) => void buy(item)} />
        {products.length > 0 && <details className="portal-purchase">
          <summary>Тестовая покупка из каталога</summary>
          <div className="clone-toolbar">
            <label>Товар
              <select value={selected} onChange={(event) => setSelected(event.target.value)}>
                <option value="">Выберите товар</option>
                {products.map((item) => <option value={item.item_id} key={item.item_id}>{item.title}</option>)}
              </select>
            </label>
            <button className="clone-link-btn clone-link-btn_primary" type="button" disabled={!selected || busy}
              onClick={() => void buy(selected)}>Купить из каталога</button>
          </div>
        </details>}
        {notice && <p role={notice.failed ? "alert" : "status"}>{notice.text}</p>}
      </section>

      <section className="flex-column gap-16 portal-overview-extra">
        <h2 className="heading-xl sl-color-text">Популярные решения</h2>
        <Fragment html={POPULAR_HTML} />
      </section>

      <section className="flex-column gap-16 portal-overview-extra">
        <h2 className="heading-xl sl-color-text">Ближайшие мероприятия</h2>
        <Fragment html={EVENTS_HTML} />
      </section>
    </div>
  );
}
