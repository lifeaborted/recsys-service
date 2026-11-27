# Модуль данных и оценки (`01-data.md`)

Модуль отвечает за воспроизводимую загрузку, валидацию, проверку обезличивания, публичный JSON-отчёт и историческую оценку рекомендательной системы (актуальная ветка доработки — `feat/data-public-metrics` поверх интегрированной `main` и `feat/data-eval`).

---

## 1. Структура зоны и файлы

| Путь | Назначение |
|---|---|
| `internal/data/csv.go` | Потоковый парсинг `clients.csv` и `sales.csv` (v4 6 колонок и v5 7 колонок с `product_group`), валидация заголовков/ссылок/дат, упаковка в `[]contract.DatasetBatch`, проверка релизов `v4`/`v5` (`419` групп в `v5`). |
| `internal/data/anonymization.go` | Проверка порогов обезличивания: `>= 3` уникальных покупателя на `item_id` и `>= 5` уникальных физлиц (`B2C`) для каждого `item_id`, встречающегося в покупках физлиц. |
| `internal/data/v4.go` | Чтение DSN из `RECSYS_DATABASE_URL` / `DATABASE_URL` (без зашитых паролей) и маскировка паролей при логировании. |
| `internal/data/data_test.go` | Модульные тесты CSV-импорта, `testdata/smoke`, пустого `sku`, v5 `product_group`, отказоустойчивости, `context.Canceled` и порогов обезличивания. |
| `internal/eval/baseline.go` | Воспроизведение эталонов `baseline.py` (`popularity`, `copurchase`, тай-брейк `score desc -> popularity desc -> item_id asc`), расчёт `Precision@3`, `FillRate`, `Full3Rate`, сводки датасета и v5 `hit_split` (`Hits`, `VariantHits`, `NewProductHits`, `VariantShare`). |
| `internal/eval/hybrid.go` | Оценка гибридного движка `contract.Recommender` в режиме `contract.Backtest` с фиксированным `SnapshotID`: только верхнеуровневые карточки (до 3), фильтр пространства 817 исторических товаров, холодные товары, разбивка по `product_group`/семействам и раскрытие утечки из будущего. |
| `internal/eval/public_report.go` | Каноническая схема публичного JSON-отчёта (`PublicMetricsReport`, `PublicMetricsDocument`) без `client_id`, `cold_items`, строк продаж и текстов объяснений, а также фиксация и валидация эталонных агрегатов `v5`. |
| `internal/eval/v4.go` | Верификация закрытых релизов `v4` и `v5` (`VerifyClosedEvaluation`, `VerifyV4Evaluation`, `CheckEvalAnonymization`). |
| `internal/eval/baseline_test.go`, `eval_test.go`, `public_metrics_test.go` | Ручные дроби, граничные тесты (корзина `(order_id, order_week)`, граница `2026-08-31` / `2026-09-07`, тай-брейк `score -> pop -> item_id`, пустой `sku`), фиксация агрегатов `v5` и валидация `public-metrics.json`. |
| `cmd/import/main.go` | CLI валидации/пакетного импорта CSV (`-dry-run`, `-check-anonymization`, `-workbook-sha256`) и верификации закрытой БД (`-verify-v5` / `-verify-v4`). |
| `cmd/backtest/main.go`, `main_test.go` | CLI исторического бэктеста на закрытой БД `v5`/`v4` или на CSV (`-label demo`), чистый публичный JSON-режим (`-format json` без лишнего вывода и `stderr`-логов) и golden-тесты на `testdata/public-eval`. |
| `testdata/smoke/`, `testdata/public-eval/` | Синтетические CSV-фикстуры (только тестовые `DEMO-*` / `SYN-*` ID) и эталонные golden JSON файлы. |
| `tests/e2e/data/public-metrics.json` | Публичный агрегатный отчёт `v5` (`"dataset": "v5"`, `"public_aggregate": true`, `visit_gap`) и синтетический пример (`"demo": true`). |

---

## 2. Книга и БД v5

Актуальный дамп `db/recommendations.sql` соответствует схеме **v5** (применены миграции `0001_init` .. `0005_v5_product_groups`):
- таблица `recommendations.dataset_release` содержит релизы `v4` (`7e219b81...`) и `v5` (`56e3c100cd600f1e0d8daf02989b3a0f82a633fa727d30aa49b4d778ef1b0c65`);
- в `recommendations.sales` присутствует заполненная колонка `product_group` (`31 753` строк, `419` уникальных групп, `0` пустых и `0` конфликтов на `item_id`);
- в `recommendations.model_snapshots` уже сохранён снимок `v5-train-20260831` (`train_through = 2026-08-31`, `33 489` кандидатов и записей `evidence_records`, без смешивания с текущим фидом). Снимок `v5-train-20260831` не пересобирается и не дополняется текущим фидом.

### Формат CSV

1. **`clients.csv`** (6 колонок):
   `client_id,client_group,client_type,industry,segment,lk_access`
   - `client_group`: строго `B2B` или `B2C`.
   - `lk_access`: `да` / `нет` (поддерживаются `true`/`false`, `1`/`0`). Клиенты с `lk_access = нет` **не удаляются** ни при импорте, ни при оценке.
   - `industry`, `segment`: опциональны (пустая строка -> `nil`).

2. **`sales.csv`** (6 колонок v4/синтетика или 7 колонок v5):
   - v4 / `testdata/smoke`: `order_id,client_id,order_week,item_id,sku,item_name`
   - v5 / `testdata/public-eval`: `order_id,client_id,order_week,item_id,sku,item_name,product_group`
   - `order_week`: дата в формате `YYYY-MM-DD`.
   - `sku`: опционально (`""` -> `nil`; в v4/v5 `11 124` таких строк).
   - Товары без совпадения в `catalog_offers` сохраняются; порядок строк и физический `SourceRow` не нарушаются.
   - `product_group`: `G<group_id>` или единый код подписки, иначе fallback на `item_id` (`419` групп на `817` уникальных `item_id`).

### Сводные размерности закрытого набора (v5)

| Показатель | Значение |
|---|---|
| Строк продаж (`sales`) | `31 753` |
| Клиентов (`clients`) | `19 008` |
| Корзин `(order_id, order_week)` | `27 603` |
| Уникальных `item_id` в продажах | `817` |
| `product_group` (в v5) | `419` |
| Товаров в `train` (`<= 2026-08-31`) | `813` |
| Холодных товаров (`test \ train`) | `4` |
| Когорта A (все / B2B / B2C) | `337` (`302` / `35`) |
| Когорта B (все / B2B / B2C) | `172` (`147` / `25`) |
| Порог обезличивания `item_id` | мин. `3` разных покупателя; мин. `5` разных B2C-покупателей для B2C-товаров |

---

## 3. Правила оценки (`internal/eval`)

1. **Сплит и корзина:** `train`: `order_week <= 2026-08-31`; `test`: `order_week > 2026-08-31` (на реальном наборе граница `2026-08-31` vs `>= 2026-09-07`). Корзина = пара `(order_id, order_week)`. Повторы одного `item_id` внутри одной корзины учитываются один раз; один `order_id` в разные недели (подписки) образует разные корзины.
2. **Когорты и знаменатель:**
   - Группа **A**: все клиенты с непустой `train`-историей и хотя бы одним новым товаром в `test \ train` (`337`).
   - Группа **B**: подмножество **A** с `len(train[client]) == 1` (`172`).
   - По правилам организаторов метрика сравнения с baseline считается строго на **всех** клиентах когорт A и B (выборка части клиентов не допускается) и на всех `817 item_id` из истории продаж, независимо от текущего `available` в фиде.
3. **Метрики и первые три позиции:**
   - В метрику `Precision@3` всегда входят только первые **3** верхнеуровневые карточки (вложенные `Variants` не добавляют слотов; при выдаче короче 3 недостающие места считаются промахом).
   - `FillRate` — доля клиентов с хотя бы 1 рекомендацией; `Full3Rate` — доля клиентов с ровно 3 рекомендациями.
   - `hit_split`: попадания делятся через `product_group` на `VariantHits` (`product_group` уже был в `train` клиента), `NewProductHits` (новая группа) и `UnknownFamilyHits` (при отсутствии `product_group` и `FamilyID`); `VariantShare` — доля вариантов от `Hits` (`%`).

---

## 4. Публичный JSON-отчёт и сводка метрик (`public-metrics.json`)

```sh
go run ./cmd/backtest -clients testdata/public-eval/clients.csv -sales testdata/public-eval/sales.csv -label demo -format json
```

В эталоне `v5` бейзлайны по всем клиентам дают `Precision@3` для популярности **5,44%** в когорте A (`337` клиентов) и **7,36%** в когорте B (`172` клиента), а для совместных покупок — **5,64%** в A и **6,01%** в B; гибрид на зафиксированном прогоне достигает **11,97%** в A (`121` попадание, из них `42` — варианты по `product_group`, **34,71%**) и **14,53%** в B (`75` попаданий, `27` вариантов, **36%**), оставляя неопубликованные подклетки гибрида в `null` (`public-metrics.json`). При расчёте метрики оцениваются только первые три позиции независимо от длины онлайн-витрины, а наблюдаемые около **55%** визитов без целевого действия отражают долю неуспешных пользовательских визитов и являются другим показателем, не равным ни `Precision@3`, ни `Fill Rate` офлайн-протокола.

---

## 5. Стыки с другими модулями

- **`internal/store/postgres` и `internal/contract`:** интеграция БД v5 (`DatasetWriter`, `EvaluationDataset` с `sales.product_group`, `OfflineDataset`, `SnapshotMetadata`) уже объединена в `main`. Контракт `Backtest` остаётся в `internal/eval/hybrid.go` без изменения чужих пакетов. Историческая БД v5 и стенд с текущим фидом / векторами не смешиваются; в снимок `v5-train-20260831` текущий фид не загружается.
- **`internal/recommend`:** в режиме `contract.Backtest` оцениваются только первые 3 карточки по `817` товарам продаж, даже при поднятии онлайн-выдачи до 36 карточек.
- **Демо (`tests/e2e/data/public-metrics.json`):** содержит опубликованные агрегаты `v5` (`"dataset": "v5"`, `"public_aggregate": true`, `"visit_gap"`) с `null` для неопубликованных полей гибрида (контрольный read-only запуск на `v5-train-20260831` дал `124` / `76` попаданий вместо опубликованных `121` / `75`, поэтому его подклетки в публичный отчёт не подмешиваются) и синтетический блок (`"demo": true`).

---

## 6. Команды запуска и тесты

Все команды выполняются из корня Go-модуля `recsys/`:

### Юнит- и golden-тесты (без БД)

```sh
go vet ./...
go test -count=1 ./internal/data/... ./internal/eval/... ./cmd/backtest/...
```

### Работа с закрытой БД `v5` (`recsys`)

```sh
RECSYS_DATABASE_URL="postgres://<user>:<password>@localhost:5432/recsys?sslmode=disable" go run ./cmd/import -verify-v5
RECSYS_DATABASE_URL="postgres://<user>:<password>@localhost:5432/recsys?sslmode=disable" go run ./cmd/backtest -dataset v5 -snapshot v5-train-20260831
RECSYS_DATABASE_URL="postgres://<user>:<password>@localhost:5432/recsys?sslmode=disable" go run ./cmd/backtest -dataset v5 -format json
```

### Импорт и бэктест CSV (`demo` / локальный `v5`)

```sh
go run ./cmd/import -clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -version demo -dry-run
go run ./cmd/backtest -clients testdata/public-eval/clients.csv -sales testdata/public-eval/sales.csv -label demo -format json
go run ./cmd/backtest -clients private/clients.csv -sales private/sales.csv -label v5 -check-anonymization
```

---

## 7. Проверенные результаты (`v5`)

- `cmd/import -verify-v5`:
  `v5 verified: sha256=56e3c100cd600f1e0d8daf02989b3a0f82a633fa727d30aa49b4d778ef1b0c65 rows=31753 clients=19008 baskets=27603 items=817 train_items=813 cold_items=4 min_buyers=3 min_b2c_buyers=5`
- Baseline `v5` (совпадает с `baseline.py` по всем 6 строкам):

| Когорта | Клиенты | Pop `P@3` / `Fill` / `Full3` / `Hits` (`Var%`) | Co `P@3` / `Fill` / `Full3` / `Hits` (`Var%`) |
|---|---:|---|---|
| `A all` | `337` | `5.44%` / `100.0%` / `100.0%` / `55` (`0.0%`) | `5.64%` / `92.3%` / `81.3%` / `57` (`10.5%`) |
| `A B2B` | `302` | `3.75%` / `100.0%` / `100.0%` / `34` (`0.0%`) | `4.53%` / `91.4%` / `79.8%` / `41` (`12.2%`) |
| `A B2C` | `35` | `20.00%` / `100.0%` / `100.0%` / `21` (`0.0%`) | `15.24%` / `100.0%` / `94.3%` / `16` (`6.2%`) |
| `B all` | `172` | `7.36%` / `100.0%` / `100.0%` / `38` (`0.0%`) | `6.01%` / `87.2%` / `69.2%` / `31` (`6.5%`) |
| `B B2B` | `147` | `5.22%` / `100.0%` / `100.0%` / `23` (`0.0%`) | `4.54%` / `85.0%` / `65.3%` / `20` (`10.0%`) |
| `B B2C` | `25` | `20.00%` / `100.0%` / `100.0%` / `15` (`0.0%`) | `14.67%` / `100.0%` / `92.0%` / `11` (`0.0%`) |

---

## 8. Безопасность

- В публичном репозитории хранятся только синтетические фикстуры (`testdata/smoke`, `testdata/public-eval`) и агрегированные отчёты (`public-metrics.json`) без `client_id`, `cold_items`, строк продаж и текстов объяснений.
- `.gitignore` исключает `*.xlsx`, `*.dump`, `*.sql.gz`, `*.yml.gz`, `recommendations*.sql`, `.env`, `.env.*`, `HANDOFF.md` в `tests/e2e/data` и приватные `*.csv`.
- Пароли и реальные DSN читаются только из переменных окружения (`RECSYS_DATABASE_URL` / `DATABASE_URL`).
