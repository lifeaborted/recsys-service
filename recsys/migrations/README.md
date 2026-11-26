# Миграции PostgreSQL

Целевая версия стенда — PostgreSQL 17 с расширениями `pg_trgm` и `vector` (`pgvector/pgvector:pg17`).

Нумерованные миграции применяются в лексикографическом порядке и отслеживаются в `public.schema_migrations`:

- `0001_init.sql` — базовая схема `recommendations` (применяется на чистой синтетической БД; если схема уже развёрнута из дампа `recommendations.sql`, шаг безопасно пропускается без изменения данных и последовательностей).
- `0002_dataset_release.sql` — таблица `recommendations.dataset_release` (`version`, `workbook_sha256`, `imported_at`).
- `0003_platform_auth_and_purchases.sql` — изолированные аккаунты стенда (`stand_accounts`), сессии с хешем токена и сроком действия (`stand_sessions`), онлайн-покупки профилей по каноническому `item_id` (`online_purchases`).
- `0004_snapshots_features_evidence.sql` — расширения `pg_trgm` и `vector`, версионирование фидов каталога (`catalog_feed_versions`, `catalog_version_offers`, `catalog_version_item_matches`) с сохранением `PRIMARY KEY (offer_id)` в `catalog_offers`, версионирование признаков (`item_feature_versions`), доказательств (`evidence_records`) и снимков рекомендаций (`model_snapshots`, `snapshot_candidates`).

Запросы из `queries.sql` предназначены для ручной аналитики и проверки данных.
