# Передача модуля 4 — PostgreSQL, векторный стенд, сессии и HTTP API

## 1. Разделение баз данных, наличие на демо и дата фида

- **Две разные базы данных.** Стенд разработки и векторного каталога (`compose.yml`, `pgvector/pgvector:pg17`) и закрытая база исторической оценки (`v4`/`v5` для `cmd/backtest`) используют разные строки подключения. В базу исторической оценки ежедневный фид и витринные эмбеддинги не загружают; фид новее `2026-08-31` не загружают в снимок `v5-train-20260831`.
- **Наличие на демо.** Наличие товаров на демо определяется флагом `available=true` из ежедневного фида и датой обновления каталога — живого остатка со склада в реальном времени нет.
- **Дата и версия фида в API.** Форма карточки `Recommendation` не изменена (новых обязательных полей не добавлено). Версия и время фида отдаются в ответах `GET /api/v1/health` и `GET /api/v1/products`:
  - `catalog_version` (из активного снимка / `recommendations.catalog_feed_versions` либо `catalog_snapshots`);
  - `feed_generated_at` (`RFC3339`);
  - `catalog_imported_at` (`RFC3339`).
  Если фид ещё не загружен, поля опускаются, а `health` на мигрированной схеме (`>= 0004`) по-прежнему отвечает `200 OK` (`status=ok`).
- **Карусель до 36 карточек.** В `api/openapi.yaml` лимит `Recommendations.items.maxItems` поднят с `3` до `36`: метрика `Precision@3` считает первые три позиции, остальные позиции предназначены только для экрана. Синтетический пример на три карточки сохранён без изменений.

## 2. Настройки окружения (`.env.example`, `Makefile`)

Все команды запускаются из каталога `recsys/`. Секреты в `.env.example` не хранятся.

| Переменная | Значение в `.env.example` | Назначение |
|---|---|---|
| `RECSYS_DEV_PG_PORT` | `55432` (фактический порт при занятом диапазоне: `15432`) | Хост-порт контейнера `postgres` из `compose.yml` |
| `DATABASE_URL` | `postgres://recsys_dev:recsys_dev@127.0.0.1:55432/recsys_dev?sslmode=disable` | DSN стенда разработки и векторного каталога (не база исторической оценки) |
| `EMBED_BASE_URL` | `http://127.0.0.1:11434/v1` | OpenAI-совместимый корень сервиса эмбеддингов |
| `EMBED_API_KEY` | пусто | Ключ API эмбеддингов (вне Git) |
| `EMBED_MODEL` | `nomic-embed-text` | Модель эмбеддингов для `cmd/embed-catalog` / `make embed-catalog` |
| `EMBED_BATCH_SIZE` | `32` | Размер пачки при векторизации офферов |
| `LLM_BASE_URL`, `LLM_MODEL` | пусто (выключены по умолчанию) | Локальная чат-модель часто не укладывается в бюджет 8 с, из-за чего запрос только ждёт таймаута |

Цель `Makefile` для индексации витринных векторов на `DATABASE_URL` стенда:

```sh
make embed-catalog ARGS="-limit 100"
```

## 3. Фактически выполненные команды и результаты

На рабочей машине стандартный порт `55432` входил в зарезервированный TCP-диапазон ОС (`55336`–`55435`), поэтому база `compose.yml` была поднята через `RECSYS_DEV_PG_PORT=15432` (`127.0.0.1:15432->5432/tcp`); чужие сервисы и порты (`3306`, `8080`, `55433`) не затрагивались.

### 3.1 Поднятие базы `compose.yml`, миграции, `health` и `seed` (PostgreSQL 17 + `pgvector`)

```powershell
cd recsys
$env:RECSYS_DEV_PG_PORT = "15432"
docker compose -p recsys-dev -f compose.yml up -d --wait postgres

$env:DATABASE_URL = "postgres://recsys_dev:recsys_dev@127.0.0.1:15432/recsys_dev?sslmode=disable"
$env:RECSYS_DATABASE_URL = $env:DATABASE_URL

# Миграции 0001..0006 на чистой базе стенда:
go run -mod=readonly ./cmd/migration up
go run -mod=readonly ./cmd/migration status

# Проверка health на чистой базе с примененной 0006:
curl.exe -s http://127.0.0.1:8080/api/v1/health
# -> 200 OK: {"status":"ok","schema_version":"0006_offer_embeddings"}

# Загрузка демо-каталога и снимка demo-v1:
go run -mod=readonly ./cmd/seed -migrate

# Проверка health и списка товаров после seed (с датой фида и available=true):
curl.exe -s http://127.0.0.1:8080/api/v1/health
# -> 200 OK: {"status":"ok","schema_version":"0006_offer_embeddings","catalog_version":"demo-feed-v1","feed_generated_at":"2026-08-31T12:00:00Z","catalog_imported_at":"..."}
```

Подтверждены расширения `pg_trgm` и `vector`, а также создание таблицы `recommendations.catalog_offer_embeddings`.

### 3.2 Повтор на PostgreSQL без расширения `vector` и проверка старых версий схемы

На экземпляре PostgreSQL без `pgvector` (`pg_available_extensions` не содержит `vector`) во временной изолированной БД выполнены:

```powershell
go run -mod=readonly ./cmd/migration up
curl.exe -i http://127.0.0.1:8080/api/v1/health
```

- Миграции `0001`–`0006` прошли целиком с кодом `0`; шаг `0006_offer_embeddings.sql` молча пропустил создание `catalog_offer_embeddings`.
- На схеме `0006_offer_embeddings` без `vector`: `GET /api/v1/health` → `200 OK` (`{"status":"ok","schema_version":"0006_offer_embeddings"}`).
- На схеме `0004_snapshots_features_evidence` (без `0005`/`0006`): `GET /api/v1/health` → `200 OK` (`{"status":"ok","schema_version":"0004_snapshots_features_evidence"}`).
- До миграций и на старой схеме `< 0004` (`0003`): `GET /api/v1/health` → `503 Service Unavailable` (`{"status":"not_ready"}`), без `500`.

### 3.3 Автотесты и сквозная проверка

```powershell
go test -count=1 ./internal/store/postgres/ ./internal/transport/http/ ./internal/auth/ ./cmd/api/
go test -mod=readonly -count=1 ./...
node --test tests/e2e/demo/contract.test.mjs
pwsh -NoProfile -File tests/e2e/platform/check.ps1
```

- Все пакеты Go (`store/postgres`, `transport/http`, `auth`, `cmd/api` и полный `./...`) — `PASS` (включая проверку `TestPlatformModule`, `CatalogMeta` и выдачи на 3 и на 36 карточек).
- `tests/e2e/demo/contract.test.mjs` — `8/8 PASS`.
- `tests/e2e/platform/check.ps1` — `PASS` (изолированный контейнер PG17 `170012`, двукратный `seed -migrate`, транзакционные тесты `pgx/v5`, сквозной сценарий `api.test.mjs` и удаление временных ресурсов).

## 4. Точки подключения (`cmd/api/main.go`, `internal/store/postgres`)

- Конструкторы хранилища: `postgres.NewPool(ctx, dsn)`, `postgres.New(pool, opts...)`, `postgres.OpenContext(ctx, dsn, opts...)`.
- В `cmd/api/main.go` `transporthttp.NewServer` получает `Auth`, `Health`, `Catalog`, `CatalogMeta`, `Purchases` и `Recommender` (`recommend.New(store)`). Поведение `internal/recommend` и фронтенда в этой ветке не изменялось.
