# Передача модуля 4 — PostgreSQL, сессии и HTTP API (`04-PLATFORM`)

## 1. Команды миграции и наполнения демо-данными (`seed`)

Все команды выполняются из каталога `recsys/`.

### На чистой синтетической БД (демо / локальная разработка)

```powershell
$env:DATABASE_URL = "postgres://postgres:postgres@localhost:5432/recsys_demo?sslmode=disable"

# Применить нумерованные миграции (0001..0004) и идемпотентно загрузить синтетику demo-v1:
go run ./cmd/seed -migrate

# Повторный запуск безопасен и не меняет счётчики записей:
go run ./cmd/seed
```

Альтернативно миграции можно применить отдельно через CLI мигратора:

```powershell
go run ./cmd/migration up
go run ./cmd/migration status
```

### Поверх существующей закрытой БД `v4` (после `psql -f recommendations.sql`)

Миграция `0001_init.sql` обнаруживает уже существующую схему `recommendations` (`recommendations.sales`) и пропускает базовый bootstrap, не затрагивая данные и последовательности дампа. Миграции `0002`–`0004` добавляют таблицы аккаунтов стенда, сессий, онлайн-покупок, версий фидов, признаков, доказательств и снимков:

```powershell
$env:DATABASE_URL = "postgres://user:pass@localhost:5432/recsys_v4?sslmode=disable"
go run ./cmd/migration up
```

`cmd/seed` по умолчанию блокирует запуск на базе с `dataset_release.version = 'v4'` (`refusing to run demo seed against closed v4 database`), защищая закрытый набор от смешивания с синтетикой.

## 2. Настройки окружения (`internal/config`, `.env.example`)

| Переменная | По умолчанию | Назначение |
|---|---|---|
| `RECSYS_ADDR` | `127.0.0.1:8080` | Адрес HTTP-сервера |
| `DATABASE_URL` | — | Основной DSN PostgreSQL (приоритет 1) |
| `RECSYS_DATABASE_URL` | — | Совместимый DSN PostgreSQL (приоритет 2) |
| `HOST`, `PORT`, `USER`, `PASSWORD`, `DATABASE` (`DATADASE`), `SSLMODE` | `localhost:5432`, `recsys`, `disable` | Полевое подключение, если `DATABASE_URL` не задан |
| `RECSYS_ENV_FILE` | — | Путь к `.env`-файлу (не переопределяет уже выставленные переменные процесса) |
| `RECSYS_REQUEST_TIMEOUT` | `10s` | Дедлайн контекста на один HTTP-запрос |
| `RECSYS_SESSION_TTL` | `24h` | Срок действия Bearer-сессии |
| `RECSYS_BCRYPT_COST` | `10` | Стоимость адаптивного хеширования пароля (`bcrypt`) |
| `RECSYS_MIGRATIONS_DIR` | `migrations` | Каталог SQL-миграций |
| `RECSYS_OPENAPI_PATH` | `api/openapi.yaml` | Путь к единому контракту OpenAPI (используется и `/swagger/`) |
| `DEMO_B2B_LOGIN` / `DEMO_B2B_PASSWORD` | `demo-b2b` / `demo-b2b-pass` | Демо-аккаунт B2B (`DEMO-BUYER`) для `cmd/seed` |
| `DEMO_B2C_LOGIN` / `DEMO_B2C_PASSWORD` | `demo-b2c` / `demo-b2c-pass` | Демо-аккаунт B2C (`DEMO-NEW`) для `cmd/seed` |

## 3. Конструкторы адаптеров и сервисов

- **Пул и PostgreSQL-хранилище (`internal/store/postgres`, `pgx/v5` + `pgxpool`)**:
  - `postgres.NewPool(ctx, dsn) (*pgxpool.Pool, error)`
  - `postgres.New(pool, opts...) *postgres.Store`
  - `postgres.OpenContext(ctx, dsn, opts...) (*postgres.Store, error)`
  - `*postgres.Store` реализует (проверено при компиляции):
    - `contract.DatasetWriter` (`WriteDataset` / `WriteDatasetWithChecksum`: staging `TEMP`-таблицы + `tx.CopyFrom`, составной ключ корзины `(order_id, order_week)` через `orders` + `order_observed_weeks` + `sales`, проверка порогов `v4` на `817`/`813` `item_id`, `31753` строк, `19008` клиентов, `27603` корзин с откатом транзакции при несовпадении, идемпотентный повтор)
    - `contract.SnapshotWriter` (`PublishSnapshot`: привязка к `dataset_release` `version` + `workbook_sha256`, сохранение `item_id` для кандидатов без оффера, атомарное переключение `is_active` в одной транзакции)
    - `contract.FactsReader` (`Profile`, `History` с учётом `throughWeek` и онлайн-покупок профиля, `Candidates` со связанными `domain.Evidence`)
    - Версионирование фидов и признаков: `PublishCatalogFeed` / `CatalogVersionOffers` (`catalog_version_offers` с PK `(catalog_version, offer_id)` без нарушения `catalog_offers_pkey (offer_id)`), `PublishItemFeatures` (`item_feature_versions`)
    - `auth.Repository` (`CreateAccountWithProfile`, `AccountByLogin`, `CreateSession`, `SessionByTokenHash`)
- **Сервис сессий и авторизации (`internal/auth`)**:
  - `auth.NewService(repo, auth.WithSessionTTL(ttl), auth.WithBcryptCost(cost), auth.WithClock(now))`
  - `auth.NewMemoryRepository()` — потокобезопасная in-memory подмена для автономных тестов без Docker.
  - Регистрация создаёт новый изолированный профиль стенда (`STAND-*`), не связываясь с обезличенными историческими `client_id` или двумя непривязанными записями в `account_users`. Пароли хешируются `bcrypt`, в БД хранится `SHA-256` токена и проверяется `expires_at`.
- **Логгер (`internal/logger`)**:
  - `logger.New(writer, slog.LevelInfo)` — JSON `log/slog` с редактированием секретов (`password`, `token`, `access_token`, `authorization`, `api_key`, `database_url` → `[REDACTED]`) и корреляцией `request_id` + `outcome`.
- **HTTP-сервер (`internal/transport/http`, `net/http`)**:
  - `transporthttp.NewServer(transporthttp.Config{Auth, Health, Catalog, Purchases, Recommender, Logger, OpenAPISpec, RequestTimeout})`
  - Один маршрут — один файл:
    - `route_health_get.go` (`GET /api/v1/health` — соединение + версия схемы `0004_snapshots_features_evidence`)
    - `route_auth_register_post.go` (`POST /api/v1/auth/register`)
    - `route_auth_login_post.go` (`POST /api/v1/auth/login`)
    - `route_users_me_get.go` (`GET /api/v1/users/me`)
    - `route_products_get.go` (`GET /api/v1/products`)
    - `route_purchases_post.go` (`POST /api/v1/purchases` — запись по каноническому `item_id` в онлайн-историю профиля сессии, `400` при попытке подмены профиля)
    - `route_recommendations_me_get.go` (`GET /api/v1/recommendations/me`)
    - `route_swagger_get.go` (`GET /swagger/` и `GET /swagger/openapi.yaml` из того же `api/openapi.yaml`)

## 4. Точка подключения общего движка (`contract.Recommender`)

В `cmd/api/main.go` HTTP-сервер получает реализацию `contract.Recommender` через поле `httpCfg.Recommender`:

```go
httpCfg.Recommender = postgres.NewRecommender(store) // заменить на recommend.New(reader) при финальной сборке
```

Обработчик `GET /api/v1/recommendations/me` зависит только от `contract.Recommender` и передаёт `contract.RecommendationQuery{ClientID: profile.ClientID, Mode: contract.Online}`.

## 5. Фактически пройденные проверки

1. **Модульные и контрактные тесты без Docker**:
   ```sh
   go test -count=1 -v ./internal/auth/... ./internal/store/postgres/... ./internal/transport/...
   ```
   - `TestAuthRegisterCreatesDistinctProfilesHashesPasswordAndExpiresTokens` — `PASS`
   - `TestValidateV4ThresholdsRejectsIncompleteDataset` — `PASS`
   - `TestPlatformModule` (`httptest`: `/health` 200/503, `/swagger/` с единым `api/openapi.yaml`, `401` без токена и после истечения TTL, `X-Request-ID` в ответах и ошибках, `400` при подмене профиля в `/purchases` и `/recommendations/me`, `404` на неизвестный товар, изоляция двух сессий `alice`/`bob` с тестовым `contract.Recommender`, отсутствие паролей и токенов в `slog`) — `PASS`

2. **Изолированная сквозная проверка на PostgreSQL 17 (`pgvector/pgvector:pg17`, Docker Compose v2, PowerShell 7)**:
   ```powershell
   pwsh -File tests/e2e/platform/check.ps1
   ```
   - Временный контейнер PostgreSQL 17 (`server_version_num=170012`) с `tmpfs`, случайным портом и уникальным project name (без подключения к рабочей БД);
   - Двукратный прогон миграций и `cmd/seed -migrate` подтвердил полную идемпотентность (`clients=2`, `stand_accounts=2`, `account_users=2`, `account_memberships=0`, `catalog_offers=8`, `catalog_version_offers=8`, `historical_items=9`, `model_snapshots=1`, `snapshot_candidates=11`, `dataset_release=1`);
   - Проверены расширения `pg_trgm` и `vector`;
   - `TestPostgresIntegration_ContractsTransactionsAndVersioning` на живом PG17 — `PASS` (транзакционный `WriteDataset` через staging/`CopyFrom`, откат неполного `v4`, несколько версий фида каталога, публикация снимка с кандидатом без оффера, чтение `Profile`/`History`/`Candidates`);
   - Сквозной HTTP-сценарий двух профилей (`DEMO-BUYER` покупает `DEMO-BASE` → видит карточку `DEMO-RENEW-12` с вариантами `12`/`24` → покупает `DEMO-RENEW-12` → купленный ID исчезает из выдачи; у `DEMO-NEW` `online_purchases = 0`) — `PASS`, ресурсы стенда удалены по завершении.

3. **Проверка совместимости с закрытой базой `v4`**:
   - Закрытый дамп `recommendations.sql` развёрнут в отдельной БД PostgreSQL (`dataset=v4`, `sha256=7e219b81f22c51a6a3060e22d7e74657034b8b1d752c3ffaa2c6fa189bb666f4`, `31753` продаж, `19008` клиентов, `27603` корзин, `817` товаров);
   - Добавочные миграции применены поверх `v4` без изменения исторических таблиц;
   - `go run ./cmd/backtest` через `pgx/v5` воспроизвёл все 6 когорт эталона (`A`: `337`, `5.44%`/`5.64%`; `B`: `172`, `7.36%`/`6.01%`).
