# Демо участника 5 — личный кабинет и проверка интерфейса

Короткая инструкция, как запустить и проверить recommendation-систему: клон личного
кабинета Softline, Go API и базу PostgreSQL со схемой `recommendations`.

## 1. Что где лежит

```
E:\github-project\recsys-service\
├── frontend-softline\   клон ЛК на React 18 + Vite 6 + TypeScript
├── recsys\              Go-агент рекомендаций: API, БД-адаптер, миграции, импорт, тесты
└── DEMO-5-README.md     этот файл
```

Демо использует отдельную синтетическую БД. Закрытую оценку v4 запускайте по [инструкции backend](recsys/README.md).

## 2. Требования

- Node.js 20+ (проверено на v26.1.0)
- Go 1.22+ (проверено на 1.26.1)
- PostgreSQL 17 с пользователем, который может создавать БД (`CREATEDB`)
- npm-зависимости фронтенда уже установлены в `frontend-softline/node_modules`

## 3. База данных

Из каталога `recsys` задайте `RECSYS_DATABASE_URL` для отдельной демо-БД и выполните:

```powershell
go run ./cmd/migration up
go run ./cmd/migration seed
```

## 4. Запуск сервисов

Терминал 1 — Go API (порт 8080):

```powershell
cd E:\github-project\recsys-service\recsys
go run ./cmd/api
```

Терминал 2 — фронтенд (порт 5173):

```powershell
cd E:\github-project\recsys-service\frontend-softline
npm run dev
```

Откройте **http://localhost:5173/**. Vite проксирует:

| Путь браузера | Куда уходит |
|---------------|-------------|
| `/api/v1/*` | `http://localhost:8080/api/v1/*` — демо-экран (Bearer) |
| `/api/*` | `http://localhost:8080/api/v1/*` — старый клиент «Тест рекомендаций» |

Если API нужно запустить без базы, на синтетических примерах:

```powershell
$env:RECSYS_MOCK = '1'
go run ./cmd/api
```

## 5. Экраны

### 5.1 Демо участника 5 — `#/demo`

Адрес: **http://localhost:5173/#/demo**. Пункт «Рекомендации (демо)» в боковом меню.

Элементы управления:

- **Профиль** — `B2B · demo-b2b` (u1) или `B2C · demo-b2c` (u2);
- **Транспорт** — `mock (синтетика)` или `http (/api/v1)`;
- **Сценарий** — авто, полная, частичная, пустая, ошибка, медленный ответ;
- кнопки «Обновить», «Новый run», «Выйти».

Что проверить:

1. B2B + авто → ровно три карточки: «Продление на 12 месяцев», «Апгрейд редакции»,
   «Дополнительный модуль».
2. В первой карточке «Сравнить варианты» → `12 мес.` и `24 мес.`, у первого —
   «Срок окончания неизвестен».
3. «Почему рекомендуем» → только связанное ребро `item:DEMO-BASE → item:DEMO-RENEW-12`.
4. Сценарий «Частичная» → баннер `insufficient_eligible_products`, одна карточка.
5. Сценарий «Пустая» → «Нет рекомендаций для этого профиля.»
6. Сценарий «Ошибка» → `Ошибка model_unavailable (request_id: demo-error-1)`.
7. Сценарий «Медленный ответ» + сразу сменить профиль → старый ответ игнорируется.
8. Транспорт `http (/api/v1)` → тот же экран получает ответ от Go API с Bearer-токеном.

### 5.2 Проверка БД — `#/recommendations`

Адрес: **http://localhost:5173/#/recommendations**.

Показывает счётчики таблиц, выбор пользователя/сценария/лимита, таблицу выдачи с
кнопками `click`/`purchase`, кнопку «Новый run» и историю run'ов. Работает с реальной
базой через `/api`.

### 5.3 Главная — `#/`

Внешний клон `lk.softline.ru`: шапка, сайдбар, промо-блоки и лента «Вам может подойти»
на данных recommendation-сервиса.

## 6. API

| Метод | Путь | Назначение |
|-------|------|------------|
| GET | `/api/v1/health` | готовность сервиса и БД |
| POST | `/api/v1/auth/login` | демо-сессия (`demo-b2b` / `demo-b2c`, пароль любой) |
| GET | `/api/v1/users/me` | текущий профиль (Bearer) |
| GET | `/api/v1/recommendations/me` | выдача для профиля (Bearer), контракт OpenAPI |
| POST | `/api/v1/recommendations/me/new` | новый run |
| POST | `/api/v1/feedback` | событие `click` / `purchase` |
| GET | `/api/v1/debug/stats`, `/api/v1/debug/runs` | служебные данные БД |
| GET | `/api/v1/recommendations?user_id=u1` | совместимость со старым клиентом ЛК |

Пример:

```powershell
curl.exe "http://localhost:8080/api/v1/recommendations?user_id=u1&limit=9"
```

## 7. Переменные окружения

Настройки лежат в `recsys/.env` (файл в `.gitignore`, не коммитится). Туда перенесены
все поля старого `config.json`: `HOST`, `PORT`, `USER`, `PASSWORD`, `DATADASE`, `SSLMODE`,
`MAINTENANCE_DB`, `SCHEMA`, `MIGRATIONS_DIR`, `MIGRATIONS_TABLE`, `RECSYS_ADDR`,
`API_MODEL_VERSION`, `API_FALLBACK_CLIENT_ID`, `API_USER_U1/U2/U3_CLIENT_ID`,
`RECSYS_EXAMPLES_DIR`, `RECSYS_MOCK`. Переменные окружения имеет приоритет над файлом.
Можно вместо полей задать сразу `RECSYS_DATABASE_URL`.

| Переменная | По умолчанию | Что задаёт |
|------------|--------------|------------|
| `RECSYS_ADDR` | `localhost:8080` | адрес Go API |
| `RECSYS_DATABASE_URL` | `postgres://postgres:postgres@localhost:5432/recsys?sslmode=disable` | подключение к БД |
| `RECSYS_MOCK` | выключен | `1`/`true` — отвечать синтетикой из `api/examples` |
| `RECSYS_EXAMPLES_DIR` | `api/examples` | каталог примеров |
| `VITE_RECSYS_API_BASE` | `/api/v1` | адрес API в браузере |
| `VITE_PROXY_RECSYS_TARGET` | `http://localhost:8080` | цель прокси Vite |

## 8. Проверки

```powershell
# Контракт примеров (без Go-сервера)
cd recsys
node --test tests/e2e/demo/contract.test.mjs     # 8 тестов, все PASS

# Go
go build ./...
go vet ./...
go test ./...

# Фронтенд
cd ..\frontend-softline
npm run build
npm run test:e2e:demo     # браузерный тест экрана, нужен запущенный npm run dev
```

Подробные шаги и результаты — в `recsys/docs/demo/README.md` и
`recsys/tests/e2e/demo/HANDOFF.md`.

## 9. Частые проблемы

| Симптом | Причина и решение |
|---------|-------------------|
| `bind: Only one usage...` на порту | уже запущен API; остановите старый процесс или смените `RECSYS_ADDR` |
| `database "recsys" does not exist` | выполните `go run ./cmd/migration up` — БД создастся автоматически |
| `relation "recommendations.*" does not exist` | не применена миграция: `go run ./cmd/migration up` |
| Пустая выдача на `#/recommendations` | проверьте демо-БД и выполните `go run ./cmd/migration seed` |
| `401 Unauthorized` на `/me` | нужен заголовок `Authorization: Bearer demo-token-u1` (или `demo-token-u2`) |
| Карточки не меняются при смене профиля | перезапустите Vite после изменения `vite.config.ts`; проверьте, что API не в mock-режиме |

Логины/пароли настоящего личного кабинета в демо не используются: все профили синтетические.
