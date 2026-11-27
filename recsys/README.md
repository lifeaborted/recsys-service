# Агент рекомендаций для личного кабинета

Go 1.26, Node.js 22+, PostgreSQL 17 и pgx. Команды выполняются из `recsys`, если не указано иное.

## Локальное демо

```powershell
make up
$env:DATABASE_URL = 'postgres://recsys_dev:recsys_dev@127.0.0.1:55432/recsys_dev?sslmode=disable'
make seed
make run
```

`make up` поднимает dev PostgreSQL с `pg_trgm` и `vector`. `make down` останавливает Compose и сохраняет том. `make seed` применяет миграции и загружает синтетику с `snapshot_id=demo-v1`. Повтор seed на синтетической БД идемпотентен. Порт API по умолчанию 8080. При использовании `RECSYS_DEV_PG_PORT` укажите этот же порт в `DATABASE_URL`. `make swagger` получает спецификацию запущенного API, интерфейс доступен на [Swagger UI](http://127.0.0.1:8080/swagger/).

Во втором терминале запустите `npm ci` и `npm run dev` из `frontend-softline`. Откройте `http://localhost:5173/#/demo`, выберите HTTP и профиль. Сценарий покупки и браузерные команды есть в [инструкции демо](docs/demo/README.md).

Материалы демо:

- Как поднять стенд по шагам — [HANDOFF платформы](tests/e2e/platform/HANDOFF.md).
- Числа A/B, доля вариантов и дата фида — [metrics.json](docs/demo/metrics.json).
- Лицензии и версии зависимостей — [THIRD_PARTY.md](THIRD_PARTY.md).
- Офлайн- и онлайн-смета — [docs/demo/cost.md](docs/demo/cost.md).
- Сценарий ролика и список кадров — [docs/demo/video.md](docs/demo/video.md).

## Офлайн-публикация

На отдельной синтетической БД с применёнными миграциями выполните:

```powershell
make import ARGS="-clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -version demo-pipeline"
make -s enrich ARGS="-feed internal/enrichment/testdata/mini-feed.xml -items internal/enrichment/testdata/mini-items.json -sources internal/enrichment/testdata/mini-sources.json -feed-url https://example.com/demo-feed.xml -catalog-version mini-v1 -snapshot pipeline-v1" > evidence.local.json
make build-signals ARGS="-database -dataset demo-pipeline -snapshot pipeline-v1 -train-through 2026-08-31 -evidence evidence.local.json"
make backtest ARGS="-clients testdata/smoke/clients.csv -sales testdata/smoke/sales.csv -label demo"
```

`import -dry-run` проверяет CSV без записи. Публикация CSV v4 или v5 требует `-workbook-sha256` исходной книги. При перенаправлении JSON используйте `make -s enrich`, чтобы Make не добавлял команду в начало файла. В `build-signals` артефакт доказательств и исторические таблицы используются до публикации активного снимка. Для покупки выбранный `offer_id` должен быть загружен в витрину с соответствием каноническому `item_id`. Мини-фид проверяет публикацию фактов, а готовую витрину для покупки создаёт `make seed`. Эмбеддинги в этих сценариях выключены.

## Модульные и сквозные проверки

| Модуль | Команда |
|---|---|
| DATA | `go test -count=1 -v ./internal/data/... ./internal/eval/...` |
| RECOMMEND | `go test -count=1 -v ./internal/signals/... ./internal/recommend/...` |
| EVIDENCE | `go test -count=1 -v ./internal/enrichment/... ./internal/explain/...` |
| PLATFORM | `go test -count=1 -v ./internal/auth/... ./internal/store/postgres/... ./internal/transport/...` |
| DEMO | `node --test tests/e2e/demo/contract.test.mjs` |

`make check` выполняет сборку, vet, unit и проверку JSON. `make test-e2e` требует Docker Compose и PowerShell 7. Каждый запуск создаёт проект, сеть, PostgreSQL на tmpfs и отдельные синтетические данные. Рабочий DSN исключается из окружения теста. [Фактические результаты](docs/demo/INTEGRATION.md).

## Закрытая оценка v5

Восстановите `recommendations-v5.sql` в отдельную пустую БД PostgreSQL 17, задайте её `DATABASE_URL` и выполните:

```sh
go run ./cmd/migration up
go run ./cmd/import -verify-v5
go run ./cmd/backtest -dataset v5 -snapshot v5-train-20260831
```

Дамп содержит готовый снимок. Чтобы построить свой, выполните `go run ./cmd/build-signals -database -dataset v5 -snapshot <новый-id> -train-through 2026-08-31` и подставьте его ID в `backtest`.

Сверяйте исходную книгу по SHA-256 из `recommendations.dataset_release`. Эталон организаторов — обновлённый `baseline.py` на книге v5. В историческом топ-3 допускаются все 817 товаров продаж вне зависимости от наличия и соответствия оффера. Когорта A содержит 337 клиентов, B — 172. Версия v5 добавляет 419 групп товаров. Попадание считается вариантом, если его `product_group` встречается среди покупок клиента в train. Закрытые входы, дамп и DSN храните локально.

В онлайн-выдаче применяются доступность, тип пользователя и тип организации из фида. Для оффера с конкретным типом организации профиль должен подтверждать этот тип. Каталожные атрибуты для демо публикуются из проверенного артефакта `enrich`.

## Пять зон работы

| № | Ветка | Зона | Первый результат |
|---|---|---|---|
| 1 | `feat/data-eval` | `cmd/import`, `cmd/backtest`, `internal/data`, `internal/eval` | Сверка baseline v5 и оценка гибридного движка. |
| 2 | `feat/recommend` | `cmd/build-signals`, `internal/signals`, `internal/recommend` | Общий `Recommend` на тестовом `FactsReader` и первые сигналы. |
| 3 | `feat/evidence` | `cmd/enrich`, `internal/enrichment`, `internal/explain` | Верифицированная связь или отличие вариантов на синтетическом каталоге. |
| 4 | `feat/platform` | `cmd/api`, `cmd/seed`, `internal/auth`, `internal/config`, `internal/logger`, `internal/transport`, `internal/store/postgres`, `migrations` | PostgreSQL-адаптер, сессия и сквозной запрос. Ведёт изменения `internal/domain`, `internal/contract` и `api/openapi.yaml`. |
| 5 | `feat/demo` | Репозиторий ЛК, `api/examples` и `docs/demo` | Карточка, варианты и граф на [синтетическом JSON](api/examples/recommendations.partial.json). |

## Стыки компонентов

- Для сравнения гибрида с baseline формируйте топ-3 из 817 исторических товаров v4. Товар из фида вне продаж занимает позицию и даёт промах. В онлайн-демо доступен весь фид с учётом `available`. Указывайте дату текущих снимков и потенциальную утечку через данные, появившиеся после отсечения.
- Base и Renewal, разные сроки и число узлов сохраняют отдельные `item_id`. Группируйте их в карточке, сохраняя отдельные ID для оценки. В отчёте по гибриду покажите доли попаданий по вариантам знакомых семейств, новым семействам и случаям без достоверного семейства.
- Открытый стенд использует синтетические профили и данные. На питче в рамках хакатона допустимы метрики, граф и псевдо-ID из v4. HTTP принадлежит тестовому стенду команды.

- `DatasetWriter` публикует нормализованные партии целиком. В v4 корзина — `(order_id, order_week)`. Недели до 31.08.2026 включительно входят в обучение. Популярность считается по разным покупателям, совместные покупки — по корзинам. Ничьи разрешаются по популярности и затем по `item_id`. Короткий список делится на 3.
- `SnapshotWriter` публикует готовый снимок, `FactsReader` читает профиль, историю и ограниченные пулы кандидатов. В историческом прогоне указывайте конкретный снимок и дату отсечения.
- HTTP и CLI бэктеста используют `recommend.New(store)` через общий контракт `Recommender`. В HTTP профиль поступает из сессии. Для истории, графа и оценки используется `item_id`, `offer_id` связывает результат с карточкой витрины.
- Для первого сквозного сценария используйте [синтетическую историю](testdata/smoke) и пример ответа. Форму публичных данных сверяйте с [OpenAPI](api/openapi.yaml). Пустые списки в ответе API представляются как `[]`, отсутствующий `partial_reason` — как `null`.

Для подключения см. `.env.example`. Приватные дампы, датасет и учётные данные держите вне открытого репозитория.
