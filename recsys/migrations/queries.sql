-- Полезные запросы для проверки рекомендательного алгоритма.
-- Все таблицы указаны с явной схемой, чтобы не зависеть от search_path.

-- 1) Витрина каталога: что вообще можно рекомендовать
SELECT offer_id, name, vendor, category, price, currency_code, available
FROM recommendations.storefront_catalog
ORDER BY price DESC NULLS LAST
LIMIT 20;

-- 2) История конкретного клиента (замените client_id)
SELECT order_id, order_week, sku, item_name, offer_id
FROM recommendations.purchase_history
WHERE client_id = 'C12808'
ORDER BY order_week DESC, excel_row
LIMIT 50;

-- 3) Самые частые покупки по всем клиентам — база для popularity-стратегии
SELECT item_name, COUNT(*) AS buys
FROM recommendations.sales
GROUP BY item_name
ORDER BY buys DESC
LIMIT 20;

-- 4) Кандидаты: товары из корзин таких же клиентов (простой signal)
WITH target_weeks AS (
  SELECT DISTINCT order_week FROM recommendations.sales WHERE client_id = 'C12808'
)
SELECT s.item_name, COUNT(DISTINCT s.client_id) AS buyers
FROM recommendations.sales s
JOIN target_weeks w USING (order_week)
WHERE s.client_id <> 'C12808'
GROUP BY s.item_name
ORDER BY buyers DESC
LIMIT 10;

-- 5) Записать результат алгоритма: сначала run, затем до трёх позиций
WITH new_run AS (
  INSERT INTO recommendations.recommendation_runs (client_id, model_version)
  VALUES ('C12808', 'hackathon-demo-1.0')
  RETURNING run_id
)
INSERT INTO recommendations.recommendation_items (run_id, rank, offer_id, source_item_id, explanation, score)
SELECT run_id, 1, '504', NULL, 'Популярно у похожих клиентов', 0.93 FROM new_run
UNION ALL
SELECT run_id, 2, '505', NULL, 'Дополняет текущие покупки', 0.87 FROM new_run
UNION ALL
SELECT run_id, 3, '506', NULL, 'Совместимо с вашим портфелем', 0.81 FROM new_run;

-- 6) Посмотреть, что показали пользователю
SELECT r.run_id, r.client_id, r.model_version, i.rank, i.offer_id,
       c.name, i.explanation, i.score
FROM recommendations.recommendation_runs r
JOIN recommendations.recommendation_items i USING (run_id)
LEFT JOIN recommendations.storefront_catalog c USING (offer_id)
ORDER BY r.requested_at DESC, i.rank;

-- 7) Обратная связь (event_type: click | purchase)
INSERT INTO recommendations.recommendation_events (run_id, offer_id, event_type)
VALUES (1, '504', 'click');

-- 8) Статистика по версиям модели
SELECT r.model_version,
       COUNT(DISTINCT r.run_id) AS runs,
       COUNT(e.event_id) FILTER (WHERE e.event_type = 'click') AS clicks
FROM recommendations.recommendation_runs r
LEFT JOIN recommendations.recommendation_events e USING (run_id)
GROUP BY r.model_version
ORDER BY runs DESC;
