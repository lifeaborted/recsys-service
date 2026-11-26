# БД рекомендательной системы (`recommendations`)

Здесь лежит дамп PostgreSQL и инструменты для работы с базой.

- `recommendations.sql` — полный `pg_dump` (PostgreSQL 17.6), схема `recommendations`, **~276 МБ**, ~260 тыс. строк данных.
- `migrations/0001_init.sql` — только DDL из дампа (30 таблиц, 2 view, 79 FK), без данных.
- `cmd/migration` — Go-команда для применения миграций; конфиг — `config.json`.
- `cmd/api` — Go REST API: фронтенд тянет рекомендации и пишет фидбек в БД.
- `queries.sql` — готовые запросы для проверки выдачи и записи рекомендаций.
- `import.ps1` — обёртка над `psql` для импорта полного дампа вместе с данными.
- `tools/extract-schema.mjs` — пересоздание `0001_init.sql` из дампа.

> Папка лежит рядом с `frontend-softline`, а не внутри неё, чтобы большой
> SQL-файл не попадал под file watcher Vite.

## Go-миграции (схема без данных)

```
bd-for-test/
  go.mod
  config.json                  подключение + путь к миграциям
  cmd/migration/main.go        CLI: up | status | down
  internal/config/config.go    загрузка JSON, ${ENV}-подстановка
  internal/migrate/migrate.go  применение SQL с учётом версий
  migrations/0001_init.sql     DDL схемы recommendations
  seeds/seed_demo.sql          минимальные демо-данные (9 офферов, клиент, валюта)
  cmd/api/main.go              REST API на :8000
  internal/api/handler.go      health / recommendations / feedback
  internal/store/store.go      запросы к recommendations.*
  cmd/import/main.go            импорт полного pg_dump без psql (COPY через lib/pq)
  tools/checkdb/main.go         быстрая проверка количества строк в таблицах
```

Применение (запускать из каталога `bd-for-test`):

```powershell
cd bd-for-test
$env:PGPASSWORD = 'пароль'
go run ./cmd/migration create-db   # создать БД, если её нет (делается автоматически)
go run ./cmd/migration status      # что применено
go run ./cmd/migration up          # применить все ожидающие миграции
go run ./cmd/migration down        # откат (нужны файлы *.down.sql)
go run ./cmd/migration seed        # демо-данные, если полный дамп ещё не залит
```

## REST API для фронтенда

```powershell
cd bd-for-test
go run ./cmd/api                   # http://localhost:8000
```

Vite уже проксирует `/api` на `http://localhost:8000` (см. `vite.config.ts`),
поэтому CORS не нужен. Фронтенд по умолчанию работает в **live**-режиме;
в `src/state/settings.tsx` можно вернуть `mockMode: true`.

| Метод | Путь | Что делает |
|-------|------|------------|
| GET | `/health` | проверка соединения, версия модели |
| GET | `/recommendations?user_id=u1&limit=9&scenario=demo` | выдача из `recommendation_runs/items` + витрины каталога |
| POST | `/recommendations/new?user_id=u1` | создать новый run (пересчитать выдачу) |
| POST | `/feedback` | пишет `click` / `purchase` в `recommendation_events` |
| GET | `/debug/stats` | количество строк по ключевым таблицам БД |
| GET | `/debug/runs?user_id=u1` | последние run'ы с товарами и числом событий |

Эти эндпоинты использует страница **«Тест рекомендаций»** (`#/recommendations`,
ссылка добавлена в боковое меню): счётчики БД, выбор пользователя/сценария/лимита,
таблица выдачи с кнопками `click` / `purchase`, кнопка «Новый run» и история run'ов.
После изменения Go-кода API нужно перезапустить (`go run ./cmd/api`).

Ответ `/recommendations` совпадает с контрактом `src/api/types.ts`:
`{ user_id, generated_at, model_version, request_id, items:[{item_id,title,...}], total }`.

При старте API проверяет `catalog_offers`: если каталог пуст, демо-данные из
`seeds/seed_demo.sql` загружаются автоматически (иначе запросы падали бы по FK
на `clients` / `catalog_offers`).

Если для клиента ещё нет `recommendation_runs`, API создаёт базовую выдачу из
`purchase_history` (топ-3 по числу покупок, `rank` 1..3) и дописывает недостающие
позиции из `storefront_catalog`. Полный дамп с реальными 135 тыс. офферов грузится
отдельно через `import.ps1`.

`config.json` → секция `api`: `listen`, `model_version`, `fallback_client_id`,
`users` (соответствие `u1/u2/u3` → `db_user_id` + `client_id`).

Из `E:\github-project\recsys-service`: `go run ./bd-for-test/cmd/migration -config bd-for-test/config.json up`.

Если базы `recsys` ещё нет, мигратор создаёт её автоматически через служебную БД
`postgres` (`database.maintenance_db`). Нужны права `CREATEDB`. Пароль можно задать
в `config.json` или через `${PGPASSWORD}`.

Применённые версии хранятся в `public.schema_migrations` (имя, sha256, время).
Каждая миграция выполняется в одной транзакции. Если файл изменился после
применения, `status` показывает `CHANGED!`. Откат работает только для файлов
`0001_init.down.sql` — их сейчас нет, поэтому `down` просто пропустит миграцию.

`config.json` поддерживает подстановку окружения, например `"password": "${PGPASSWORD}"`.

## Импорт полного дампа (вместе с данными)

Нужен `psql` **17.x** (в дампе есть `\restrict` / `\unrestrict` — старые клиенты их не поймут).

```powershell
cd bd-for-test
$env:PGPASSWORD = 'ваш_пароль'
.\import.ps1 -Host localhost -Port 5432 -User postgres -Database recsys
```

Или напрямую:

```bash
psql -h localhost -p 5432 -U postgres -d recsys -f bd-for-test/recommendations.sql
```

База должна существовать заранее; скрипт создаёт схему `recommendations` и все таблицы.

Можно импортировать дамп и без `psql` — Go-утилитой (читает COPY-блоки и грузит их
через `pq.CopyIn`, 260 тыс. строк за ~20 сек):

```powershell
cd bd-for-test
go run ./cmd/import -truncate
go run ./cmd/import -truncate -only=catalog_offers,clients -max-rows=5000
```

`-truncate` очищает таблицы перед загрузкой; `-only` и `-max-rows` нужны для
быстрого теста на части данных. Импорт временно отключает проверку FK в сессии
(нужны права суперпользователя), так как pg_dump выдаёт таблицы не в том порядке,
в котором создавались внешние ключи.

## Схема (основное)

| Группа | Таблицы | Что хранят |
|--------|---------|------------|
| Каталог | `catalog_snapshots`, `catalog_offers`, `catalog_categories`, `catalog_vendors`, `catalog_groups`, `catalog_currencies` | офферы YML-фида Softline, цены, картинки, параметры (jsonb) |
| Витрина | view `storefront_catalog` | оффер + категория + вендор в одном ряду |
| Клиенты | `clients`, `account_profiles`, `account_users`, `account_memberships` | компании, пользователи, роли (сейчас memberships пуста) |
| Заказы | `orders`, `order_observed_weeks`, `sales`, `store_order_items`, `order_invoices`, `order_payments`, `order_deliveries`, `order_documents`, `licenses` | история покупок, структурированные заказы, счета, лицензии |
| История | `historical_items`, `historical_item_offers`, view `purchase_history` | сопоставление исторических SKU с офферами каталога |
| Корзина | `carts`, `cart_items` | корзины и их статус |
| Рекомендации | `recommendation_runs`, `recommendation_items`, `recommendation_events` | **куда писать результаты алгоритма** (сейчас пусто) |
| Excel-заглушки | `readme`, `validation_protocol`, `current_block`, `catalog_notes` | исходные листы из вашего файла |

Ограничения, важные для рекомендаций:

- `recommendation_items.rank` — только **1..3**;
- `recommendation_events.event_type` — только `click` / `purchase`;
- `orders.source` — `history` / `store` / `account`;
- `account_memberships.role` — `owner` / `admin` / `buyer` / `viewer`.

## Объём данных в дампе

| Таблица | Строк |
|---------|-------|
| `catalog_offers` | 135 890 |
| `clients` | 19 162 |
| `sales` | 32 279 |
| `orders` | 27 878 |
| `order_observed_weeks` | 27 929 |
| `catalog_categories` | 9 311 |
| `catalog_vendors` | 1 234 |
| `historical_item_offers` | 729 |
| `historical_items` | 1 079 |
| `account_users` | 2 |
| `recommendation_runs` / `items` / `events` | 0 |

Пользователи в БД:

| user_id | email | display_name |
|---------|-------|--------------|
| 2 | `471hattie@sharebot.net` | Обычный тестовый пользователь |
| 3 | `wukehiro@ozvmail.com` | Верифицированный тестовый клиент |

`account_memberships` пуста — прямую связку `user_id → client_id` надо выбрать вручную
(например, взять `client_id` из `sales` для теста). В клоне на фронтенде пока
используются условные `u1/u2/u3`; при подключении реальной БД их нужно заменить
на `user_id` 2/3 и передавать `client_id` в запросе рекомендаций.

## Связь с фронтендом

Vite проксирует `/api` на `VITE_PROXY_TARGET` (по умолчанию `http://localhost:8000`).
Там должен подняться сервис с эндпоинтами `GET /recommendations`, `POST /feedback`,
`GET /health` — контракт описан в корневом `README.md`. Пока такого сервиса нет,
фронтенд работает в mock-режиме.
