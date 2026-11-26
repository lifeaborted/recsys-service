# Передача — участник 5 (ЛК и проверка интерфейса)

## Репозитории и версии

| Проект | Путь | Версия |
|--------|------|--------|
| recsys | `E:\github-project\recsys-service\recsys` | module `recsys`, go 1.22; сборка на Go 1.26.1 |
| ЛК (frontend) | `E:\github-project\recsys-service\frontend-softline` | `frontend-softline@0.1.0`, React 18 + Vite 6 + TS 5.7 |

Git CLI в окружении отсутствует, поэтому номер ревизии заменить списком изменённых
файлов ниже. Node.js v26.1.0 (требуется 20+).

## Зона записи в recsys

- `api/examples/recommendations.full.json` — полная выдача, 3 карточки, граф с двумя концами у каждого ребра.
- `api/examples/recommendations.partial.json` — частичная (`partial_reason`), 1 карточка DEMO-RENEW.
- `api/examples/recommendations.empty.json` — пустая выдача, `items: []`, `partial_reason: null`.
- `api/examples/recommendations.error.json` — ошибка с `request_id` и `error.code/message`.
- `docs/demo/README.md` — команды, адрес экрана, шаги и ожидаемые надписи.
- `tests/e2e/demo/contract.test.mjs` — автономный тест на `node:test` + `assert`.
- `tests/e2e/demo/HANDOFF.md` — этот файл.

## Изменения в репозитории ЛК

- `src/api/recsysTypes.ts`, `src/api/recsysClient.ts` — типы и клиент по `api/openapi.yaml`, два транспорта.
- `src/api/examples/*.json` — копии синтетических примеров для подмены транспорта.
- `src/state/session.tsx` — демо-сессии B2B/B2C, `generation` для игнорирования запоздалых ответов.
- `src/hooks/useDemoRecommendations.ts` — загрузка, отмена, состояния и `request_id`.
- `src/components/DemoRecommendationCard.tsx` — карточка с объяснением, ссылкой, вариантами и графом.
- `src/pages/DemoPage.tsx`, `src/lib/routes.ts`, `src/App.tsx`, `tools/gen-fragments.mjs` — экран `#/demo` и ссылка в меню.
- `src/styles/clone.css`, `vite.config.ts` — стили карточек и прокси `/api/v1 → :8080`.

## Переключение подмены на API

По умолчанию «Транспорт = mock (синтетика)» — клиент читает `src/api/examples`.
На экране `#/demo` выберите «Транспорт = http (/api/v1)»; тот же клиент начнёт
вызывать `GET /api/v1/recommendations/me` с `Authorization: Bearer demo-token-u1`
(или `demo-token-u2`). Вместо адреса в UI можно задать `VITE_RECSYS_API_BASE`,
а прокси — `VITE_PROXY_RECSYS_TARGET`. Сервер поднимается командой:

```sh
cd recsys
$env:RECSYS_MOCK='1'
go run ./cmd/api     # :8080, /api/v1/*
```

## Фактические результаты

### JSON (без Go-сервера)

```
node --test tests/e2e/demo/contract.test.mjs
tests 8, pass 8, fail 0
```

Проверено: полная/частичная/пустая/ошибочная выдача; не более трёх карточек;
оба конца каждого ребра есть в `graph.nodes`; все `evidence_ids` разрешаются;
варианты DEMO-RENEW различаются сроками 12 и 24 месяца; неизвестный срок сохранён
как `null`; ошибка содержит `request_id`.

### Браузер

Проверено во встроенном браузере на `http://localhost:5173/#/demo` (раздаётся Vite):

- baseline (B2B, mock, авто): 3 карточки, `request_id: demo-full-1`;
- «Сравнить варианты»: `DEMO-RENEW-12 · 12 мес.`, `DEMO-RENEW-24 · 24 мес.`,
  надпись «Срок окончания неизвестен»;
- «Почему рекомендуем»: только связанное ребро `item:DEMO-BASE → item:DEMO-RENEW-12`;
- частичная выдача: баннер `insufficient_eligible_products`, 1 карточка, `demo-1`;
- пустая: «Нет рекомендаций для этого профиля.», 0 карточек;
- ошибка: `Ошибка model_unavailable (request_id: demo-error-1)`, 0 карточек;
- медленный ответ + смена профиля: через 1,9 с остаётся выдача B2C (`demo-1`),
  запоздалый ответ B2B не применён; «Выйти» очищает данные;
- HTTP-транспорт через `/api/v1` при запущенном Go API: показана 1 карточка B2C,
  консоль без ошибок.

Отдельного браузерного раннера в стеке ЛК нет, поэтому проверка выполнена вручную
по шагам `docs/demo/README.md`; один тест примеров не является доказательством
подключения экрана — HTTP-транспорт проверен отдельно, как описано выше.

## Замечание по зонам

По просьбе заказчика дополнительно перенесена рабочая реализация в зону участника 4:
`cmd/api`, `internal/auth`, `internal/config`, `internal/store/postgres`,
`internal/transport/http/**`, а также `internal/contract`/`internal/domain` не
изменялись. Владельцу зоны 4 — проверить совместимость с общим `Recommender` и
OpenAPI перед финальной сборкой. Утилиты `cmd/migration`, `cmd/import`, SQL-миграция,
seed и `queries.sql` перенесены в `recsys`; временная папка `bd-for-test/` удалена.
Закрытый дамп v4 восстанавливается в отдельную чистую БД по [инструкции](../../../README.md).
