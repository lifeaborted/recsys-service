# Передача — участник 1 (Данные и оценка, `01-DATA.md`)

## Ветка, окружение и зона записи

- **Ветка:** `feat/data-eval`
- **Версия Go:** `go 1.22` в `go.mod`, проверено на `go1.26.5`
- **Зона записи:** `cmd/import`, `cmd/backtest`, `internal/data`, `internal/eval`, `tests/e2e/data`
- **Защита от утечек:** добавлен корневой `.gitignore` и расширен `recsys/.gitignore` (исключены `*.xlsx`, `*.dump`, `*.sql.gz`, `*.yml.gz`, `recommendations.sql`, `.env`, `.env.*` и приватные `*.csv` с явным разрешением для `testdata/**`).

---

## Что реализовано

1. **`internal/data` (`csv.go`, `anonymization.go`, `v4.go`, `data_test.go`):**
   - Чтение `clients.csv` и `sales.csv` через `encoding/csv` с `context.Context`, проверкой заголовков, уникальности `client_id`, `client_group` (`B2B`/`B2C`), `lk_access` (`да`/`нет`), формата `order_week` (`YYYY-MM-DD`) и внешней ссылки `sales.client_id -> clients.client_id`.
   - Сохранение исходного порядка строк, `SourceRow` (`2, 3, ...`), пустого `SKU` (`nil`) и товаров без оффера в каталоге.
   - Упаковка в упорядоченные партии (`[]contract.DatasetBatch`, сначала клиенты, затем продажи) и передача в `contract.DatasetWriter` (валидация выполняется целиком до вызова `WriteDataset`).
   - Проверка порогов обезличивания v4 (`CheckAnonymization`): `>= 3` уникальных покупателя на `item_id` и `>= 5` уникальных физлиц (`B2C`) для любого `item_id`, встречающегося у `B2C`.
   - Сверка закрытого релиза v4 (`VerifyV4Release`): версия `v4`, SHA-256 книги `7e219b81f22c51a6a3060e22d7e74657034b8b1d752c3ffaa2c6fa189bb666f4`, размеры `31 753` строки продаж, `817` товаров.
2. **`internal/eval` (`baseline.go`, `hybrid.go`, `v4.go`, `baseline_test.go`, `eval_test.go`):**
   - Сплит по `order_week <= 2026-08-31` (train) / `> 2026-08-31` (test), корзина `(order_id, order_week)`, дедупликация позиций внутри корзины, сохранение клиентов с `lk_access = false` и товаров без оффера.
   - Расчёт обоих baseline (`popularity` и `copurchase`) с тай-брейком `score desc -> popularity desc -> item_id asc`, `Precision@3` (знаменатель всегда `3`), `FillRate` и `Full3Rate` (доля полных троек).
   - Оценка гибрида через `contract.Recommender` (`Mode: contract.Backtest`, фиксированный `SnapshotID`) и локальный интерфейс `eval.Backtest`:
     - учёт только верхнеуровневых карточек `Items` (до 3, вложенные `Variants` не занимают слоты);
     - поддержка ограничения пространства 817 исторических `item_id` (`AllowedItems`: товар из фида вне 817 занимает позицию и даёт промах);
     - отдельный учёт холодных товаров (`test \ train`, 4 товара в v4);
     - разбивка попаданий по семействам (`KnownFamilyHits`, `NewFamilyHits`, `UnknownFamilyHits` от общего числа попаданий);
     - предупреждения о потенциальной утечке из будущего (`DefaultLeakageWarnings`).
   - Реализован `TestDataModule` со всеми требуемыми подслучаями.
3. **`cmd/import` и `cmd/backtest`:**
   - `cmd/import` переведён на CSV-импорт через `internal/data` + `contract.DatasetWriter` (вместо временного загрузчика `pg_dump`) и поддерживает режим `-verify-v4` для проверки закрытой БД.
   - `cmd/backtest` поддерживает как закрытую БД v4, так и автономный режим `-clients/-sales` с меткой `demo`, выводом `Full3Rate`, сводкой `813/4` и раскрытием утечки будущего.

---

## Стыки для участника 4 (`feat/platform`) и участника 2 (`feat/recommend`)

### 1. Семантика `contract.DatasetWriter` (для участника 4)
- **Атомарность и откат:** `WriteDataset(ctx, version, batches)` должен публиковать все партии одной версии `version` в единой транзакции PostgreSQL (через staging-таблицы/`COPY`). При ошибке валидации, разрыве соединения или отмене `ctx` транзакция откатывается, а предыдущая опубликованная версия и активный снимок остаются рабочими.
- **Повторная загрузка (идемпотентность):** повторный вызов `WriteDataset` с тем же `version` и теми же данными должен завершаться без дублирования строк в `recommendations.sales` и `recommendations.clients` и сохранять порядок `excel_row` (`SourceRow`).
- **Целостность данных:** не отфильтровывать записи с `SKU == nil`, без совпадения в `historical_item_offers` и клиентов с `HasLKAccess == false`.
- **Метаданные релиза:** для v4 фиксировать `(version, workbook_sha256)` в `recommendations.dataset_release`.

### 2. Необходимые расширения `internal/contract` и `internal/store/postgres` (для участника 4)
Чужие пакеты (`internal/contract`, `internal/domain`, `internal/store/postgres`) на этапе `feat/data-eval` не изменялись. Для сквозной интеграции (`COMPOSE.md`) потребуется:
1. Добавить в `internal/contract` интерфейс `Backtest` (сейчас зеркально определён в `internal/eval/hybrid.go`):
   ```go
   type Backtest interface {
       Run(ctx context.Context, rec Recommender, cfg BacktestConfig) (HybridReport, error)
   }
   ```
2. Расширить адаптер оценки в `internal/store/postgres` (`EvaluationV4` или отдельный метод `BacktestContext`):
   - сверять `workbook_sha256` с `7e219b81f22c51a6a3060e22d7e74657034b8b1d752c3ffaa2c6fa189bb666f4` внутри SQL-адаптера;
   - возвращать `catalog_snapshots.feed_generated_at` для отчёта об актуальности каталога и рисках утечки;
   - возвращать `clients` (включая клиентов без продаж в `sales`), `historical_items` (817 ID для `AllowedItems`) и сопоставление `item_id -> family_id` для разбивки попаданий.
3. Подключить `postgres.DatasetWriter` в `cmd/import` (сейчас без `-dry-run` и `-verify-v4` команда явно сообщает об ожидании SQL-адаптера от зоны 4).
4. Согласовать имя переменной окружения БД: `.env.example` использует `DATABASE_URL`, а `internal/config/config.go` — `RECSYS_DATABASE_URL` (в `cmd/import` и `cmd/backtest` реализован фоллбэк `RECSYS_DATABASE_URL -> DATABASE_URL` без зашитых паролей).

### 3. Требования к движку `contract.Recommender` (для участника 2)
- При `Mode: contract.Backtest` использовать только переданный `SnapshotID` (обученный строго на `order_week <= 2026-08-31`) и возвращать до 3 верхнеуровневых карточек `Items`.
- Для честного сравнения с baseline v4 формировать топ-3 только из 817 исторических `item_id` (товары фида вне этого списка считаются промахом в слоте).
- Заполнять `Recommendation.FamilyID` (либо `nil`, если семейство не подтверждено), чтобы `eval.EvaluateRecommender` строил честную разбивку попаданий на знакомые варианты, новые продукты и неизвестное семейство.

---

## Команды CLI и флаги

### `cmd/import`
- Проверка CSV и формирование батчей без БД:
  ```sh
  go run ./cmd/import -clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -version demo -dry-run
  ```
- Проверка закрытой БД v4 (`RECSYS_DATABASE_URL` или `DATABASE_URL` из окружения):
  ```sh
  go run ./cmd/import -verify-v4
  ```
- Флаги: `-clients`, `-sales`, `-version` (по умолчанию `demo-v1`), `-batch-size` (`1000`), `-check-anonymization`, `-dry-run`, `-verify-v4`.

### `cmd/backtest`
- Закрытая проверка на PostgreSQL v4:
  ```sh
  go run ./cmd/backtest
  ```
- Автономная проверка синтетики / CSV (с пометкой `demo`):
  ```sh
  go run ./cmd/backtest -clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -label demo
  ```
- Флаги: `-clients`, `-sales`, `-label`, `-check-anonymization`, `-show-cold-items` (по умолчанию `false`), `-format` (`table` | `json`).

**Ожидаемые столбцы таблицы `cmd/backtest`:**
`Cohort`, `Group`, `Clients`, `Pop_P@3`, `Pop_Fill`, `Pop_Full3`, `Co_P@3`, `Co_Fill`, `Co_Full3`.

---

## Фактические результаты проверок

### 1. Модульные тесты (`internal/data` и `internal/eval`)
```sh
go test -count=1 -v ./internal/data/... ./internal/eval/...
```
Результат: **PASS** (`ok recsys/internal/data`, `ok recsys/internal/eval`), включая:
- `TestImportSmokeFixture` (импорт 2 клиентов и 1 продажи из `testdata/smoke`)
- `TestImportCSVPreservesOrderEmptySKUAndNoOffer`
- `TestImportCSVValidationAndFailClosed`
- `TestCheckAnonymization`
- `TestWeeklyBasketsAndShortList`
- `TestTieBreakByPopularityThenItemID`
- `TestDataModule` (`CohortsAAndBManualFractions`, `EmptyListAndZeroNeighbors`, `EmptySKUAndBoundaryDates`, `SameOrderIDDifferentWeeksAndUniqueBuyers`, `DeterministicUnderRowPermutation`, `HybridRecommenderAndFamilies`, `SmokeFixtureImportAndEval`)

### 2. Синтетика `testdata/smoke` (CLI)
```text
import dry-run ok: version=demo clients=2 sales=1 batches=2 items=1 empty_sku=0
dataset=demo (synthetic/demo) train_through=2026-08-31 test_from=>2026-08-31
summary: rows=1 clients=1 baskets=1 items=1 train_items=1 cold_items=0 cohort_a=0 cohort_b=0
```

### 3. Закрытая сверка v4 (`recsys` на PostgreSQL, дамп v4)
```text
v4 verified: sha256=7e219b81f22c51a6a3060e22d7e74657034b8b1d752c3ffaa2c6fa189bb666f4 rows=31753 clients=19008 baskets=27603 items=817 train_items=813 cold_items=4 min_buyers=3 min_b2c_buyers=5

dataset=v4 sha256=7e219b81f22c51a6a3060e22d7e74657034b8b1d752c3ffaa2c6fa189bb666f4 train_through=2026-08-31 test_from=>2026-08-31
summary: rows=31753 clients=19008 baskets=27603 items=817 train_items=813 cold_items=4 cohort_a=337 cohort_b=172
Cohort  Group  Clients  Pop_P@3  Pop_Fill  Pop_Full3  Co_P@3  Co_Fill  Co_Full3
A       all        337    5.44%    100.0%     100.0%   5.64%    92.3%     81.3%
A       B2B        302    3.75%    100.0%     100.0%   4.53%    91.4%     79.8%
A       B2C         35   20.00%    100.0%     100.0%  15.24%   100.0%     94.3%
B       all        172    7.36%    100.0%     100.0%   6.01%    87.2%     69.2%
B       B2B        147    5.22%    100.0%     100.0%   4.54%    85.0%     65.3%
B       B2C         25   20.00%    100.0%     100.0%  14.67%   100.0%     92.0%
```

Совпадает с эталоном `baseline.py` на v4 по всем шести строкам (`A=337: 5.44% / 5.64%`, `B=172: 7.36% / 6.01%`).
