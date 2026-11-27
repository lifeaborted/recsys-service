# Агент рекомендаций для личного кабинета

Прототип показывает покупателю B2B или B2C выдачу рекомендаций до 36 карточек (в метрику входят первые три), объясняет выбор и помогает сравнить варианты продукта. В демо можно купить базовую лицензию, получить рекомендации на апгрейд и дополнение. Для оценки качества есть бэктест на отложенных покупках.

Проект собран на Go 1.26, PostgreSQL 17, React 18, TypeScript и Vite. Демо работает с собственным API команды.

## Запуск двух сервисов в Docker Compose

У каждого сервиса свой каталог с `Dockerfile` и `docker-compose.yml`. Для отдельного запуска перейдите в каталог сервиса. Обе команды ниже начинаются из корня репозитория. Параметр `-t 10` задаёт таймаут остановки в секундах.

**HTTP-заглушка** ([frontend-softline](frontend-softline))

```sh
cd frontend-softline
docker compose up -d --build -t 10
curl -fsS http://127.0.0.1:8080/api/v1/health
```

Compose в `frontend-softline` запускает HTTP-заглушку из общего Go-модуля. В режиме `stub` она работает на синтетических примерах. Для проверки можно войти с демо-профилем `demo-b2b` и паролем `demo-b2b-pass`.

**gRPC-сервис рекомендаций** ([recsys](recsys))

Задайте `DATABASE_URL` существующей PostgreSQL с применёнными миграциями и активным снимком. Адрес в DSN должен быть доступен из контейнера. Для PostgreSQL на машине, где работает Docker Engine, можно использовать `host.docker.internal` при доступе к базе с интерфейса Docker.

```sh
export DATABASE_URL='postgres://user:password@host.docker.internal:5432/recsys?sslmode=disable'
cd recsys
docker compose up -d --build -t 10
docker compose ps
```

На синтетической БД со снимком `demo-v1` проверка из корня репозитория выглядит так.

```sh
echo '{"clientId":"DEMO-BUYER","limit":3}' |
  grpcurl -plaintext -import-path recsys -proto api/recommendation/v1/recommendation.proto -d @ 127.0.0.1:50051 recsys.recommendation.v1.RecommendationService/GetRecommendations
```

Для проверки цепочки HTTP → gRPC на той же БД выполните команду с двумя файлами из корня репозитория. Compose запустит сервисы в общей сети. Если эти порты уже заняты отдельными экземплярами, выполните `docker compose down` в каталогах сервисов.

```sh
RECSYS_MOCK=0 docker compose -p recsys-demo \
  -f recsys/docker-compose.yml \
  -f frontend-softline/docker-compose.yml \
  up -d --build -t 10
```

Опубликованные порты привязаны к `127.0.0.1`. Для запуска на занятом порту задайте `RECSYS_HTTP_PORT` или `RECSYS_GRPC_PORT`. Внутри Compose HTTP API обращается к `recommend:50051`. Этот локальный стенд использует plaintext gRPC. Настройки mTLS для отдельных систем приведены в [README сервиса](recsys/README.md#grpc-для-других-систем).

## Быстрый запуск

Понадобятся Go 1.26+, Node.js 22+, GNU Make и Docker с Compose v2. Запустите команды в трёх терминалах из корня репозитория. Для синтетического демо достаточно файлов из Git.

**Терминал 1. Сервис рекомендаций (PowerShell)**

Подготовьте отдельную демо-базу PostgreSQL и укажите её адрес в `DATABASE_URL`.

```powershell
cd recsys
$env:DATABASE_URL = 'postgres://recsys_dev:recsys_dev@127.0.0.1:55432/recsys_dev?sslmode=disable'
make seed
make run-recommend
```

В Bash и Zsh задайте ту же переменную через `export DATABASE_URL='…'`. `make seed` применяет миграции и загружает тестовый каталог, профили и снимок `demo-v1`. Для синтетики подойдёт PostgreSQL 17 с расширением `pg_trgm`.

**Терминал 2. HTTP API (PowerShell)**

```powershell
cd recsys
$env:DATABASE_URL = 'postgres://recsys_dev:recsys_dev@127.0.0.1:55432/recsys_dev?sslmode=disable'
make run
```

API вызывает отдельный сервис рекомендаций по gRPC на `127.0.0.1:50051`. Для других систем доступен [контракт Protobuf](recsys/api/recommendation/v1/recommendation.proto) и [пример вызова](recsys/README.md#grpc-для-других-систем).

**Терминал 3. Интерфейс**

```sh
cd frontend-softline
npm ci
npm run dev
```

Откройте [демо](http://localhost:5173/#/demo), выберите транспорт `http (/api/v1)` и профиль B2B. Купите `DEMO-BASE`, проверьте объяснение апгрейда. После покупки апгрейда его карточка исчезнет из выдачи. Переключитесь на B2C, чтобы увидеть выдачу второго профиля с собственной историей.

API доступен по адресу `http://127.0.0.1:8080`. Для проверки подключения откройте [health](http://127.0.0.1:8080/api/v1/health). Контракт и интерактивные запросы доступны в [OpenAPI](recsys/api/openapi.yaml) и [Swagger UI](http://127.0.0.1:8080/swagger/). Для нестандартного адреса Go API передайте `VITE_PROXY_RECSYS_TARGET` при запуске Vite. [Подробный сценарий демо](recsys/docs/demo/README.md)

Материалы демо: [метрики v5](recsys/tests/e2e/data/public-metrics.json), [сторонние компоненты](recsys/THIRD_PARTY.md), [смета](recsys/docs/demo/cost.md), [сценарий ролика](recsys/docs/demo/video.md).

## Как устроен проект

| Компонент | Что делает | Где смотреть |
|---|---|---|
| Данные и оценка | Проверяет и импортирует CSV. Считает бейзлайны популярности и совместных покупок, сравнивает их с гибридом. | [Модуль данных и оценки](recsys/tests/e2e/data/README.md) |
| Сигналы и ранжирование | Строит снимок сигналов и использует его вместе с профилем, историей и правилами совместимости для выдачи. | [Движок рекомендаций](recsys/tests/e2e/recommend/HANDOFF.md) |
| Каталог и объяснения | Читает YML, сопоставляет позиции, проверяет цитаты и связи и передаёт доказательства для карточек. | [Обогащение каталога](recsys/tests/e2e/evidence/HANDOFF.md) |
| API и PostgreSQL | Хранит сессии, покупки, версии данных и снимки. Отдаёт каталог и рекомендации текущего профиля. | [Backend](recsys/README.md) · [Платформа](recsys/tests/e2e/platform/HANDOFF.md) |
| Интерфейс | Показывает каталог, покупки, карточки, причины и варианты. Поддерживает HTTP и синтетическую подмену. | [Frontend](frontend-softline/README.md) |

Поток обработки начинается с CSV и, при наличии проверенных источников, каталожного YML. Команды `import` и `enrich` готовят данные и доказательства, а `build-signals` публикует снимок в PostgreSQL. API и исторический бэктест используют общий движок. API выбирает активный снимок, исторический прогон закрепляет конкретный ID. `item_id` обозначает товар для ранжирования и оценки, `offer_id` связывает его с предложением витрины.

Адаптер эмбеддингов покрыт тестами. В интегрированном демо и историческом прогоне векторный поиск пока выключен. [Отчёт об интеграции](recsys/docs/demo/INTEGRATION.md)

## Проверки

Из `recsys` выполните:

```sh
make check
make test-e2e
```

`make check` выполняет сборку Go, `go vet`, unit-тесты и проверку ответов демо. `make test-e2e` требует Docker Compose и PowerShell 7. Он запускает API и PostgreSQL на отдельном синтетическом стенде. С уже запущенным чистым демо-API можно отдельно выполнить `node --test tests/e2e/demo/api.test.mjs`. Этот тест записывает покупки, поэтому используйте свежую синтетическую базу.

Из `frontend-softline` выполните `npm ci` и `npm run build`. Для браузерного теста команды запуска, параметры Chromium и режим записи покупок описаны в [инструкции демо](recsys/docs/demo/README.md).

## Историческая оценка

Обучение заканчивается неделей `2026-08-31` включительно. Корзина определяется парой `(order_id, order_week)`. Основные метрики — Precision@3 и Fill Rate для когорт A и B с разбивкой по B2B и B2C. Топ-3 для сравнения ограничен товарами из продаж. Текущие атрибуты каталога, доступность и профили могут содержать сведения после отсечения.

- **v5 в PostgreSQL.** Команда `import -verify-v5` сверяет релиз и 419 групп товаров. `build-signals -dataset v5` готовит снимок, а `backtest -snapshot …` сравнивает гибрид с бейзлайнами и считает долю попаданий по знакомым группам. [Команды закрытого прогона](recsys/README.md#закрытая-оценка-v5)
- **v5 в CSV.** Передайте `backtest -clients <clients.csv> -sales <sales.csv> -label v5` для автономного расчёта бейзлайнов. [Формат входов и правила оценки](recsys/tests/e2e/data/README.md)

Открытый стенд использует синтетику. Книги организаторов, выгрузки из закрытого набора, дампы и ключи хранятся в закрытом контуре команды.
