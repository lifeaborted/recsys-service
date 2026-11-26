# Интеграционный прогон 26.11.2026

Исходная `main` — `5869f10`. Все участники объединены последовательными merge в `codex/integrate-main`; последний merge исходных веток — `bad050a`. После интеграционного коммита `main` обновляется fast-forward. Содержимое пяти исходных веток доступно в истории:

| Модуль | Входная ревизия | Материалы |
|---|---|---|
| DATA | `bde0428` | Локальный промпт `01-DATA.md`. В этой ревизии `tests/e2e/data/HANDOFF.md` снят с отслеживания; использован [README модуля](../../tests/e2e/data/README.md). |
| RECOMMEND | `798669d` | `02-RECOMMEND.md` и [HANDOFF](../../tests/e2e/recommend/HANDOFF.md). |
| EVIDENCE | `1c1173c` | `03-EVIDENCE.md` и [HANDOFF](../../tests/e2e/evidence/HANDOFF.md). |
| PLATFORM | `4107c83` | `04-PLATFORM.md` и [HANDOFF](../../tests/e2e/platform/HANDOFF.md). |
| DEMO | `8da0e27` | `05-DEMO.md` и [HANDOFF](../../tests/e2e/demo/HANDOFF.md). |

В конфликтах серверных файлов использованы pgx, сессии и OpenAPI из PLATFORM. Клиент DEMO переведён на Bearer-сессию и канонический `item_id`. Старые страницы главной и отладки используют свои mock-данные по умолчанию.

## Команды и результаты

Команды Go и Node из таблицы, кроме браузерной и фронтенд-сборки, выполнены из `recsys`.

| Проверка | Команда | Код | Результат |
|---|---|---:|---|
| DATA | `go test -count=1 -v ./internal/data/... ./internal/eval/...` | 0 | CSV, корзина по неделе, две когорты, гибрид и семейства прошли. |
| RECOMMEND | `go test -count=1 -v ./internal/signals/... ./internal/recommend/...` | 0 | Отсечение, движок, добор после фильтрации прошли. |
| EVIDENCE | `go test -count=1 -v ./internal/enrichment/... ./internal/explain/...` | 0 | Синтетика SKU, YML, проверка цитаты и таймаут прошли. Тест эмбеддингов использовал локальную подмену. Эмбеддинги на стенде и в оценке отключены. |
| PLATFORM | `go test -count=1 -v ./internal/auth/... ./internal/store/postgres/... ./internal/transport/...` | 0 | Транзакционные тесты pgx на отдельной нативной БД и HTTP-контракт прошли. `internal/transport/http/respond` — `[no test files]`. |
| DEMO | `node --test tests/e2e/demo/contract.test.mjs` | 0 | 8 из 8. |
| Сборка, vet, unit | `make check` | 0 | Все пять модулей имеют тесты. `[no test files]` у `cmd/{api,backtest,build-signals,enrich,import,migration,seed}`, `internal/{contract,domain,migrate,transport/http/respond}`, `tools/checkdb`. PG-интеграция пропускается в обычном unit-прогоне и проверена отдельно по строке PLATFORM. |
| Чистая БД и повтор seed | Дважды `go run ./cmd/seed -migrate` | 0 / 0 | PostgreSQL 17.6, `pg_trgm`. После каждого вызова: 2 клиента, 2 аккаунта, 8 офферов, 1 снимок, 11 кандидатов. |
| Офлайн-цепочка | `cmd/import`, `make -s enrich`, `cmd/build-signals -database` с аргументами из [README](../../README.md) | 0 / 0 / 0 | pgx записал 2 клиента и 1 продажу. Активен `pipeline-v1`, одно подтверждённое ребро, версия фида `mini-v1` от 26.11.2026. |
| HTTP на PostgreSQL 17 | `RECSYS_E2E_API=http://127.0.0.1:18080 node --test tests/e2e/demo/api.test.mjs` | 0 | Семь операций, Swagger, граф, изоляция профилей, 401/400, эхо `X-Request-ID`, покупка и честная частичная выдача. Снимок `demo-v1`. |
| Swagger | `make swagger API_URL=http://127.0.0.1:18080` | 0 | Отдан OpenAPI 3.0.3. |
| Сборка ЛК | `npm ci`, `npm run build` | 0 / 0 | TypeScript и Vite собрали production-артефакт. |
| Экран ЛК | `npm run test:e2e:demo` с `RECSYS_E2E_LIVE=1` | 0 | 10 из 10, 0 пропусков. Edge, `http://127.0.0.1:15173/#/demo`, API `http://127.0.0.1:18081`, `demo-v1`. Отдельная чистая синтетическая БД. Через UI куплен `DEMO-BASE`, показаны варианты 12/24 и доказательство, затем куплен 12-месячный вариант и его ID исчез. |
| Валидация Compose | `docker compose -f compose.yml config --quiet` и аналогично для `tests/e2e/platform/docker-compose.yml` в WSL | 0 / 0 | Обе конфигурации разобраны Compose. |
| Изолированный Compose | `make test-e2e` | 1 | Не завершён в этом окружении. См. `INT-DOCKER-01`. |

## Закрытая оценка v4

Дамп восстановлен в отдельную пустую БД. `cmd/migration up`, `cmd/import -verify-v4`, `cmd/build-signals -database -dataset v4 -snapshot v4-integration-20260831`, `cmd/backtest -snapshot v4-integration-20260831` и `python baseline.py <книга-v4>` завершились с кодом 0. SHA-256 книги совпал с `recommendations.dataset_release`. В наборе 31 753 продажи, 19 008 клиентов, 27 603 корзины и 817 товаров. Train заканчивается неделей 31.08.2026 включительно. Корзина — `(order_id, order_week)`. После отсечения онлайн-покупки и каталог в обучение не попадают. Профили из дампа могут отражать более позднее состояние.

Go и Python совпали по числу клиентов, Precision@3 и FillRate во всех шести строках. В таблице проценты, в колонке гибрида также число попаданий. Топ-3 гибрида ограничен 817 товарами продаж.

| Когорта | Клиенты | Популярность P@3 / Fill | Совместные P@3 / Fill | Гибрид P@3 / попадания |
|---|---:|---:|---:|---:|
| A all | 337 | 5.44 / 100.0 | 5.64 / 92.3 | 11.97 / 121 |
| A B2B | 302 | 3.75 / 100.0 | 4.53 / 91.4 | 11.26 / 102 |
| A B2C | 35 | 20.00 / 100.0 | 15.24 / 100.0 | 18.10 / 19 |
| B all | 172 | 7.36 / 100.0 | 6.01 / 87.2 | 14.53 / 75 |
| B B2B | 147 | 5.22 / 100.0 | 4.54 / 85.0 | 13.61 / 60 |
| B B2C | 25 | 20.00 / 100.0 | 14.67 / 100.0 | 20.00 / 15 |

Для всех строк гибрида доли по трём категориям составили 0% вариантов знакомых семейств, 0% подтверждённых новых семейств и 100% неизвестных семейств. В историческом снимке использованы сигналы продаж. Полного версионного фида с подтверждёнными семействами на дату отсечения в локальных входах нет. Текущие каталог и наличие потребуют отдельной маркировки сведений из будущего.

## Реестр сбоев

| ID | Компонент и воспроизведение | Ожидание | Наблюдение и первый лог | Статус |
|---|---|---|---|---|
| INT-BUILD-01 | Сборка сразу после merge, `go test -mod=readonly ./...` | Все команды компилируются | `cmd/migration/main.go:48:53: cfg.MaintenanceDB undefined` | Исправлен стык конфигурации. `make check` проходит. |
| INT-BROWSER-01 | Старый экран ЛК, `npm run test:e2e:demo` | Главная показывает карточки | `page.waitForFunction: Timeout 30000ms exceeded` при запросе прежнего API | Для старых экранов включены mock-данные по умолчанию. Браузер отдельно проверяет живой OpenAPI на `/#/demo`. |
| INT-EVAL-01 | `go run ./cmd/backtest -snapshot v4-integration-20260831` | Счётчик попаданий совпадает с Precision@3 и разбивкой | `A all clients=337 P@3=11.97% hits=0` | Исправлена агрегация. У A 121, у B 75 попаданий. |
| INT-DOCKER-01 | Docker Compose, `make test-e2e`; в WSL `docker pull pgvector/pgvector:pg17` | Запуск PG17 и проверка обоих расширений | Windows: `The term 'docker' is not recognized`. В WSL образ начал загружаться (`Pulling fs layer`), готовность контейнера не получена. | Открыт. HTTP, браузер и pgx проверены на чистом PG17 с `pg_trgm`; эта установка не содержит `vector`. |
| INT-V4-01 | Семейства, `go run ./cmd/backtest -snapshot v4-integration-20260831` | Доли на подтверждённых исторических семействах | `variant=0.00% new=0.00% unknown=100.00%`. В локальном закрытом наборе нет верифицированного каталожного снимка на дату отсечения. | Открыт по входным данным. Baseline и исторический прогон по продажам проверены. |

Сквозной Compose-прогон предназначен для хоста с Docker Compose и PowerShell 7. Он создаёт уникальный проект и сеть, запускает PostgreSQL на tmpfs с синтетическим DSN и удаляет ресурсы проекта. Открытые тесты используют только синтетику, закрытые входы и учётные данные в репозиторий не добавлены.
