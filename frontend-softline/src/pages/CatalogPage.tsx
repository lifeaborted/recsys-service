import { useMemo, useState } from "react";
import type { RecommendationItem } from "../api/types";
import { RecommendationCard } from "../components/RecommendationCard";
import { CATALOG } from "../data/catalog";
import { useDebug } from "../state/debug";

export function CatalogPage() {
  const [query, setQuery] = useState("");
  const { showScores } = useDebug();

  const items = useMemo<RecommendationItem[]>(() => {
    const needle = query.trim().toLowerCase();
    return CATALOG.filter((item) =>
      !needle ||
      `${item.title} ${item.vendor} ${item.sku} ${item.tags.join(" ")}`.toLowerCase().includes(needle),
    ).map((item) => ({
      item_id: item.id,
      title: item.title,
      subtitle: item.vendor,
      category: item.category,
      score: item.popularity / 100,
      reason: `каталог · популярность ${item.popularity}`,
      price: item.price,
      currency: "RUB",
      image_url: item.image,
      url: item.url,
    }));
  }, [query]);

  return (
    <div className="react-page-inner">
      <h1>Продукты</h1>
      <p className="react-page__lead">
        Каталог из снимка личного кабинета. Поиск фильтрует локальные данные.
      </p>
      <div className="clone-toolbar">
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder="Название продукта, вендор или артикул"
        />
      </div>
      <div className="clone-product-grid">
        {items.map((item) => (
          <RecommendationCard key={item.item_id} item={item} showScores={showScores} />
        ))}
        {!items.length ? <div className="reco-empty">Ничего не найдено</div> : null}
      </div>
    </div>
  );
}
