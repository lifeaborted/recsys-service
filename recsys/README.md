# Агент рекомендаций для личного кабинета

Стартовый каркас Go-проекта. Для сборки нужен Go 1.22 или новее. Каркас собирается на стандартной библиотеке. Целевое хранилище — существующая схема `recommendations` в PostgreSQL 17.

```sh
go build ./...
go test ./...
go vet ./...
go run ./cmd/api
```

`GET http://127.0.0.1:8080/api/v1/health` возвращает `ok`, если схема `recommendations`
доступна (в mock-режиме — всегда `ok`). Адрес задаётся `RECSYS_ADDR`, подключение —
`RECSYS_DATABASE_URL`. Подмена транспорта: `RECSYS_MOCK=1` раздаёт синтетику из `api/examples`.

Работа с БД и импорт дампа:

```sh
go run ./cmd/migration up
go run ./cmd/migration seed
go run ./cmd/import -file E:\recommendations.sql -truncate
```

Демо-экран личного кабинета и передача участника 5 — в `docs/demo/README.md` и
`tests/e2e/demo/HANDOFF.md`. Остальные операции описаны в черновике OpenAPI.

## Пять зон работы

| № | Ветка | Зона | Первый результат |
|---|---|---|---|
| 1 | `feat/data-eval` | `cmd/import`, `cmd/backtest`, `internal/data`, `internal/eval` | Чтение v3 и воспроизведение baseline. Для первого закрытого прогона данные уже есть в PostgreSQL. |
| 2 | `feat/recommend` | `cmd/build-signals`, `internal/signals`, `internal/recommend` | Общий `Recommend` на тестовом `FactsReader` и первые сигналы. |
| 3 | `feat/evidence` | `cmd/enrich`, `internal/enrichment`, `internal/explain` | Верифицированная связь или отличие вариантов на синтетическом каталоге. |
| 4 | `feat/platform` | `cmd/api`, `cmd/seed`, `internal/auth`, `internal/config`, `internal/logger`, `internal/transport`, `internal/store/postgres`, `migrations` | PostgreSQL-адаптер, сессия и сквозной запрос. Ведёт изменения `internal/domain`, `internal/contract` и `api/openapi.yaml`. |
| 5 | `feat/demo` | Репозиторий ЛК, `api/examples` и `docs/demo` | Карточка, варианты и граф на [синтетическом JSON](api/examples/recommendations.partial.json). |

## Стыки компонентов

- `DatasetWriter` публикует нормализованные партии целиком. В v3 `sales.excel_row` задаёт порядок первого появления, `sales.order_week` — границу train/test. При отсечении по 31.08.2026 используйте неделю строки продажи.
- `SnapshotWriter` публикует готовый снимок, `FactsReader` читает профиль, историю и ограниченные пулы кандидатов. В историческом прогоне указывайте конкретный снимок и дату отсечения.
- HTTP и бэктест вызывают один `Recommender`. В HTTP профиль поступает из сессии. Для истории, графа и оценки используется `item_id`, `offer_id` связывает результат с карточкой витрины.
- Для первого сквозного сценария используйте [синтетическую историю](testdata/smoke) и пример ответа. Форму публичных данных сверяйте с [OpenAPI](api/openapi.yaml). Пустые списки в ответе API представляются как `[]`, отсутствующий `partial_reason` — как `null`.

Используйте `.env.example` как перечень будущих настроек. На этом этапе сервер читает из окружения только `RECSYS_ADDR`. Приватные дампы, датасет и учётные данные держите вне открытого репозитория.
