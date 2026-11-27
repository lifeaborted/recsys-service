# Миграции PostgreSQL

Целевая версия стенда — PostgreSQL 17 с расширениями `pg_trgm` и `vector` (`pgvector/pgvector:pg17`).

Нумерованные миграции применяются в лексикографическом порядке и отслеживаются в `public.schema_migrations`:

- `0001_init.sql` — базовая схема `recommendations` (применяется на чистой синтетической БД; если схема уже развёрнута из дампа `recommendations.sql`, шаг безопасно пропускается без изменения данных и последовательностей).
- `0002_dataset_release.sql` — таблица `recommendations.dataset_release` (`version`, `workbook_sha256`, `imported_at`).
- `0003_platform_auth_and_purchases.sql` — изолированные аккаунты стенда (`stand_accounts`), сессии с хешем токена и сроком действия (`stand_sessions`), онлайн-покупки профилей по каноническому `item_id` (`online_purchases`).
- `0004_snapshots_features_evidence.sql` — расширение `pg_trgm` (и `vector` при наличии на сервере), версионирование фидов каталога (`catalog_feed_versions`, `catalog_version_offers`, `catalog_version_item_matches`) с сохранением `PRIMARY KEY (offer_id)` в `catalog_offers`, версионирование признаков (`item_feature_versions`), доказательств (`evidence_records`) и снимков рекомендаций (`model_snapshots`, `snapshot_candidates`).
- `0005_v5_product_groups.sql` — колонка `product_group` в `sales` и `organization_type` в `item_feature_versions`.
- `0006_offer_embeddings.sql` — таблица `recommendations.catalog_offer_embeddings` для векторного поиска по каталогу (создаётся при наличии расширения `vector` и молча пропускается на сборке PostgreSQL без `pgvector`).

`CheckHealth` считает схему рабочей, когда применены миграции не старше `0004_snapshots_features_evidence` (включая `0006_offer_embeddings`). На более старой или неприменённой схеме `/api/v1/health` возвращает `503` (`status=not_ready`), не `500`.
