# Модуль данных и оценки (`01-DATA.md`)

Модуль отвечает за воспроизводимую загрузку, валидацию, проверку обезличивания и историческую оценку рекомендательной системы (ветка `feat/data-eval`).

---

## 1. Структура зоны и файлы

| Путь | Назначение |
|---|---|
| `internal/data/csv.go` | Потоковый парсинг `clients.csv` и `sales.csv` (v4 6 колонок и v5 7 колонок с `product_group`), валидация заголовков/ссылок/дат, упаковка в `[]contract.DatasetBatch`, вызов `contract.DatasetWriter`, проверка релизов `v4`/`v5`. |
| `internal/data/anonymization.go` | Проверка порогов обезличивания: `>= 3` уникальных покупателя на `item_id` и `>= 5` уникальных физлиц (`B2C`) для каждого `item_id`, встречающегося в покупках физлиц. |
| `internal/data/v4.go` | Чтение DSN из `RECSYS_DATABASE_URL` / `DATABASE_URL` (без зашитых паролей) и маскировка паролей при логировании. |
| `internal/data/data_test.go` | Модульные тесты CSV-импорта, [testdata/smoke](../../testdata/smoke), пустого `sku`, v5 `product_group`, отказоустойчивости, `context.Canceled` и порогов обезличивания. |
| `internal/eval/baseline.go` | Воспроизведение эталонов `baseline.py` (`popularity`, `copurchase`, тай-брейк `score desc -> popularity desc -> item_id asc`), расчёт `Precision@3`, `FillRate`, `Full3Rate`, сводки датасета и v5 `hit_split` (`Hits`, `VariantHits`, `NewProductHits`, `VariantShare`). |
| `internal/eval/hybrid.go` | Оценка гибридного движка `contract.Recommender` в режиме `contract.Backtest` с фиксированным `SnapshotID` и локальным интерфейсом `eval.Backtest`: только верхнеуровневые карточки (до 3), фильтр пространства 817 исторических товаров, холодные товары, разбивка по семействам и раскрытие потенциальной утечки из будущего. |
| `internal/eval/v4.go` | Проверка закрытой базы v4 (`VerifyV4Evaluation`, `CheckEvalAnonymization`). |
| `internal/eval/baseline_test.go`, `internal/eval/eval_test.go` | Набор тестов `TestDataModule` и регрессий baseline/гибрида с ручными дробями. |
| `cmd/import/main.go` | CLI валидации/пакетного импорта CSV (`-dry-run` / `-check-anonymization`) и верификации закрытой БД (`-verify-v4` / `-verify-v5`). |
| `cmd/backtest/main.go` | CLI исторического бэктеста на закрытой БД v4/v5 или на CSV-файлах v5 / синтетики (`-label demo`). |

---

## 2. Книга и БД v5

Дамп `recommendations-v5.sql` включает `recommendations.sales.product_group`, метаданные релиза и исторический снимок. PostgreSQL принимает официальный импорт v5 через CSV с SHA-256 книги. Команды проверки и бэктеста приведены в [README](../../README.md#закрытая-оценка-v5). Ниже v4 упоминается для формата прежнего релиза и сравнения с ним.

### Формат CSV

1. **`clients.csv`** (6 колонок):
   `client_id,client_group,client_type,industry,segment,lk_access`
   - `client_group`: строго `B2B` или `B2C`.
   - `lk_access`: `да` / `нет` (также поддерживаются `true`/`false`, `1`/`0`).
   - `industry`, `segment`: опциональны (пустая строка -> `nil`).
   - Клиенты с `lk_access = нет` **не удаляются** ни при импорте, ни при оценке.

2. **`sales.csv`** (поддерживаются оба формата — 6 колонок v4/синтетика и 7 колонок v5):
   - v4 / `testdata/smoke`: `order_id,client_id,order_week,item_id,sku,item_name`
   - v5: `order_id,client_id,order_week,item_id,sku,item_name,product_group`
   - `order_week`: дата понедельника недели в формате `YYYY-MM-DD`.
   - `sku`: опционально (`""` -> `nil`; в v4/v5 `11 124` таких строк).
   - Товары без совпадения в `catalog_offers` сохраняются.
   - `SourceRow` сохраняет физический номер строки (`2, 3, ...`), порядок строк не нарушается.
   - `product_group` (колонка G в v5):
     - `G<group_id>` из YML-фида для сопоставленных товаров;
     - единый код тарифа для помесячных подписок Google Workspace;
     - `item_id` для прочих товаров (или если колонка G пуста/отсутствует, как в v4 и `baseline.py`).
     - В v5 на `817` уникальных `item_id` приходится **`419`** уникальных `product_group` (`0` пустых, `0` конфликтов `item_id -> product_group`).

### Сводные размерности закрытого набора (v4 / v5)

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
| Порог обезличивания `item_id` (факт) | мин. `3` разных покупателя; мин. `5` разных B2C-покупателей для B2C-товаров |

---

## 3. Правила оценки (`internal/eval`)

1. **Сплит и корзина:**
   - `train`: `order_week <= 2026-08-31`; `test`: `order_week > 2026-08-31` (в закрытом наборе недели между `2026-08-31` и `2026-09-07` отсутствуют; в граничных тестах `> 2026-08-31` попадает в `test` как в `baseline.py`).
   - Корзина = пара `(order_id, order_week)`. Один `order_id` в разные недели (подписки) образует разные корзины. Повторы одного `item_id` внутри одной корзины учитываются один раз.
2. **Когорты и истина:**
   - Истина клиента = множество новых для него товаров `test[client] \ train[client]`.
   - Группа **A**: клиенты с непустой `train`-историей и непустым множеством новых товаров в `test`.
   - Группа **B**: подмножество A с `len(train[client]) == 1`.
3. **Метрики:**
   - `Precision@3` = среднее по клиентам группы от `hits / 3` (неполная выдача < 3 не уменьшает знаменатель — недостающие слоты считаются промахом).
   - `FillRate` = доля клиентов с хотя бы 1 рекомендацией.
   - `Full3Rate` = доля клиентов с ровно 3 рекомендациями.
   - `hit_split` (v5 `baseline.py`): среди попаданий считается `VariantHits` (чьё эффективное `product_group` уже встречалось в `train`-истории клиента) и `NewProductHits` (новое `product_group`), выводится `VariantShare` (`%` от `Hits`).
4. **Оценка гибрида (`EvaluateRecommender` / `eval.Backtest`):**
   - Вызывает `contract.Recommender.Recommend` с `Mode: contract.Backtest` и закреплённым `SnapshotID`.
   - Учитываются только верхнеуровневые карточки `Items` (максимум 3; вложенные `Variants` не дают дополнительных слотов).
   - По подтверждённым правилам организаторов главная таблица сравнения с baseline считается строго на всех **817 `item_id`** из продаж, **независимо от текущего `available` и сопоставления с фидом** (`AllowedItems`; товар фида вне 817 занимает позицию и даёт промах). Ограничение `available="true"` применяется только в демо-выдаче.
   - Отдельно считаются цели и попадания по холодным товарам (`ColdItemStats`).
   - Разбивка попаданий по семействам: при наличии канонической колонки `ProductGroups` (v5 `product_group`) она имеет приоритет над `FamilyID` карточки (вариант vs новый продукт совпадает с `baseline.py`); при отсутствии `product_group` и `FamilyID == nil` такие попадания учитываются отдельно как `UnknownFamilyHits`.
   - Всегда добавляются предупреждения об утечке из будущего (`DefaultLeakageWarnings`: отсутствие исторических снимков каталога, `available` и профилей на `2026-08-31`).

---

## 4. Стыки с другими модулями (для интеграции)

### Для зоны 4 (`internal/contract`, `internal/store/postgres`)
1. **Семантика `contract.DatasetWriter`:**
   - Публиковать все `DatasetBatch` одной версии `version` в **единой транзакции** (staging/`COPY`): при ошибке или отмене `ctx` транзакция откатывается, прежняя версия остаётся в силе.
   - Повторный вызов с тем же `version` должен быть **идемпотентным** (без дублирования строк и с сохранением `excel_row` = `SourceRow`).
   - Не отбрасывать `SKU == nil`, товары без матча в `historical_item_offers` и клиентов с `HasLKAccess == false`.
   - Хранить `(version, workbook_sha256)` в `recommendations.dataset_release`.
2. **Контракт `Backtest`:** перенести интерфейс `eval.Backtest` (`Run(ctx, rec, cfg) (HybridReport, error)`) из `internal/eval/hybrid.go` в `internal/contract` при финальной сборке.
3. **Расширение `EvaluationV4` / снимков БД:** для v5 используется хранение/чтение `sales.product_group`, передачу `catalog_snapshots.feed_generated_at`, списка `historical_items` (817) и подтверждённых семейств в адаптер оценки.
4. **Переменная БД:** `.env.example` использует `DATABASE_URL`, а `internal/config/config.go` — `RECSYS_DATABASE_URL`. Наши CLI читают `RECSYS_DATABASE_URL` с фоллбэком на `DATABASE_URL`.

### Для зоны 2 (`internal/recommend`)
- Реализация `contract.Recommender` в режиме `contract.Backtest` должна опираться строго на снимок `SnapshotID` (обученный на `order_week <= 2026-08-31`), возвращать до 3 верхнеуровневых карточек из всех **817 исторических `item_id`** независимо от текущего `available` и наличия оффера в фиде (а в онлайн-демо — использовать доступные товары фида с проверкой ограничений «Пользователь» и «Тип организации»). При работе с v4/без `product_group` заполнять `FamilyID`; в v5 каноническую классификацию варианта/нового продукта обеспечивает колонка `product_group` в `BacktestConfig.ProductGroups`.
- Примечание по итогам Q&A: экспорт ручных связок текущего блока ЛК не предоставляется — все связи строятся без них.

---

## 5. Команды запуска и тесты

Все команды выполняются из корня Go-модуля `recsys/`:

### Юнит- и модульные тесты (без БД)
```sh
go vet ./...
go build ./...
go test -count=1 -v ./internal/data/... ./internal/eval/...
```

### Импорт и проверка CSV (без БД / с проверкой порогов)
```sh
# Синтетика из testdata/smoke
go run ./cmd/import -clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -version demo -dry-run

# Локальные приватные CSV v5 (в .gitignore) с проверкой порогов обезличивания
go run ./cmd/import -clients private/clients.csv -sales private/sales.csv -version v5 -check-anonymization -dry-run

# Проверка загруженной закрытой БД v4
RECSYS_DATABASE_URL="postgres://<user>:<password>@localhost:5432/recsys?sslmode=disable" go run ./cmd/import -verify-v4
```

### Бэктест (синтетика `demo`, CSV v5 и закрытая БД v4)
```sh
# Синтетика (помечается dataset=demo)
go run ./cmd/backtest -clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -label demo

# Закрытая проверка на локальных v5 CSV (полный пересчёт с product_group)
go run ./cmd/backtest -clients private/clients.csv -sales private/sales.csv -label v5 -check-anonymization

# Закрытая проверка на БД v4 (формат table или json)
RECSYS_DATABASE_URL="postgres://<user>:<password>@localhost:5432/recsys?sslmode=disable" go run ./cmd/backtest
RECSYS_DATABASE_URL="postgres://<user>:<password>@localhost:5432/recsys?sslmode=disable" go run ./cmd/backtest -format json
```

**Столбцы отчёта `cmd/backtest`:**
`Cohort`, `Group`, `Clients`, `Pop_P@3`, `Pop_Fill`, `Pop_Full3`, `Pop_Hits`, `Pop_Var%`, `Co_P@3`, `Co_Fill`, `Co_Full3`, `Co_Hits`, `Co_Var%`.

---

## 6. Фактические результаты проверок (агрегаты)

### 1. Тесты пакетов
`go test -count=1 -v ./internal/data/... ./internal/eval/...` — **PASS** (`TestImportSmokeFixture`, `TestImportCSVPreservesOrderEmptySKUAndNoOffer`, `TestImportCSVV5ProductGroupColumn`, `TestImportCSVValidationAndFailClosed`, `TestCheckAnonymization`, `TestWeeklyBasketsAndShortList`, `TestTieBreakByPopularityThenItemID`, `TestDataModule` со всеми 8 подслучаями).

### 2. Синтетика `testdata/smoke`
```text
import dry-run ok: version=demo clients=2 sales=1 batches=2 items=1 product_groups=1 has_product_group=false empty_sku=0
dataset=demo has_product_group=false train_through=2026-08-31 test_from=>2026-08-31
summary: rows=1 clients=1 baskets=1 items=1 product_groups=1 train_items=1 cold_items=0 cohort_a=0 cohort_b=0
```

### 3. Закрытая БД `recsys` (дамп v4)
```text
v4 verified: sha256=7e219b81f22c51a6a3060e22d7e74657034b8b1d752c3ffaa2c6fa189bb666f4 rows=31753 clients=19008 baskets=27603 items=817 train_items=813 cold_items=4 min_buyers=3 min_b2c_buyers=5
```

### 4. Закрытый прогон v5 (`Датасет_Рекомендации_ЛК_v5.xlsx`, сверено с `baseline.py`)
```text
dataset=v5 has_product_group=true train_through=2026-08-31 test_from=>2026-08-31
summary: rows=31753 clients=19008 baskets=27603 items=817 product_groups=419 train_items=813 cold_items=4 cohort_a=337 cohort_b=172
Cohort  Group  Clients  Pop_P@3  Pop_Fill  Pop_Full3  Pop_Hits  Pop_Var%  Co_P@3  Co_Fill  Co_Full3  Co_Hits  Co_Var%
A       all        337    5.44%    100.0%     100.0%        55      0.0%   5.64%    92.3%     81.3%       57    10.5%
A       B2B        302    3.75%    100.0%     100.0%        34      0.0%   4.53%    91.4%     79.8%       41    12.2%
A       B2C         35   20.00%    100.0%     100.0%        21      0.0%  15.24%   100.0%     94.3%       16     6.2%
B       all        172    7.36%    100.0%     100.0%        38      0.0%   6.01%    87.2%     69.2%       31     6.5%
B       B2B        147    5.22%    100.0%     100.0%        23      0.0%   4.54%    85.0%     65.3%       20    10.0%
B       B2C         25   20.00%    100.0%     100.0%        15      0.0%  14.67%   100.0%     92.0%       11     0.0%
```

---

## 7. Безопасность

- В публичном репозитории хранятся только синтетические фикстуры (`testdata/smoke`) и агрегированные метрики.
- `.gitignore` в корне и в `recsys/` блокирует `*.xlsx`, `*.dump`, `*.sql.gz`, `*.yml.gz`, `recommendations.sql`, `.env`, `.env.*` и любые приватные `*.csv` (кроме `testdata/**`).
- Пароли и реальные DSN не хранятся в коде и коммитах — используйте переменные окружения (`RECSYS_DATABASE_URL` / `DATABASE_URL`) или локальный `.env`.
