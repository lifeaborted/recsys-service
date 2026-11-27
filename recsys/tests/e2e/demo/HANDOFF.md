# Передача — участник 5 (ЛК и проверка интерфейса)

## Репозитории и версии

| Проект | Путь | Версия |
|--------|------|--------|
| recsys | `E:\github-project\recsys-service\recsys` | module `recsys`, go 1.26.0; сборка на Go 1.26.1 |
| ЛК (frontend) | `E:\github-project\recsys-service\frontend-softline` | `frontend-softline@0.1.0`, React 18 + Vite 6 + TS 5.7 |

Git CLI в окружении отсутствует, поэтому номер ревизии заменить списком изменённых
файлов ниже. Node.js v26.1.0 (требуется 20+).

## Зона записи в recsys

- `api/examples/recommendations.full.json` — полная выдача, 3 карточки, граф с двумя концами у каждого ребра.
- `api/examples/recommendations.partial.json` — частичная (`partial_reason`), 1 карточка дополнительного модуля.
- `api/examples/recommendations.empty.json` — пустая выдача, `items: []`, `partial_reason: null`.
- `api/examples/recommendations.error.json` — ошибка с `request_id` и `error.code/message`.
- `api/examples/recommendations.verified.json` — 2 карточки: подтверждённый апгрейд и дополнение со статусом `review`; без продления.
- `docs/demo/metrics.json` — числа A/B, доля вариантов, 55% визитов, дата фида и оговорка про каталог; `"fallback": true`.
- `docs/demo/README.md` — команды, адрес экрана, шаги и ожидаемые надписи.
- `docs/demo/cost.md`, `docs/demo/video.md` — смета и сценарий ролика.
- `THIRD_PARTY.md` — версии и лицензии зависимостей.
- `tests/e2e/demo/contract.test.mjs` — автономный тест на `node:test` + `assert`.
- `tests/e2e/demo/HANDOFF.md` — этот файл.

## Изменения в репозитории ЛК

- `src/api/recsysTypes.ts`, `src/api/recsysClient.ts` — типы и клиент по `api/openapi.yaml`, два транспорта.
- `src/api/examples/*.json` — копии синтетических примеров для подмены транспорта.
- `src/state/session.tsx` — демо-сессии B2B/B2C, синтетический профиль без истории `u3` и `generation` для игнорирования запоздалых ответов.
- `src/hooks/useDemoRecommendations.ts` — загрузка, отмена, состояния и `request_id`.
- `src/components/DemoRecommendationCard.tsx` — карточка с объяснением, ссылкой, вариантами, бейджем проверки и графом.
- `src/components/DemoGraph.tsx` — граф только по рёбрам ответа API.
- `src/components/DemoGraphModal.tsx` — модальное окно графа (Escape, фон, кнопка × в углу).
- `src/components/DemoMetricsPanel.tsx` — таблица A/B, варианты, 55% визитов, дата фида и оговорка.
- `src/components/DemoQualityChart.tsx` — полосы сравнения A и B под таблицей, без внешних библиотек.
- `src/pages/DemoPage.tsx`, `src/lib/routes.ts`, `src/App.tsx`, `tools/gen-fragments.mjs` — экран `#/demo` и ссылка в меню.
- `src/styles/clone.css`, `vite.config.ts` — стили карточек и прокси `/api/v1 → :8080`.

## Переключение подмены на API

По умолчанию «Транспорт = mock (синтетика)» — клиент читает `src/api/examples`.
На экране `#/demo` выберите «Транспорт = http (/api/v1)»; клиент выполнит
`POST /api/v1/auth/login` и подставит полученный токен в
`Authorization: Bearer …` для `GET /api/v1/recommendations/me`. Профиль
`demo-cold (без истории)` доступен только на mock и переключает транспорт сам.
Вместо адреса в UI можно задать `VITE_RECSYS_API_BASE`, а прокси —
`VITE_PROXY_RECSYS_TARGET`. Сервер поднимается командой:

```sh
cd recsys
$env:RECSYS_ENV_FILE='.env'   # или задайте DATABASE_URL
go run ./cmd/api              # :8080, /api/v1/*
```

## Фактические результаты

### JSON (без Go-сервера)

```
node --test tests/e2e/demo/contract.test.mjs
tests 10, pass 10, fail 0
```

Проверено: полная/частичная/пустая/ошибочная выдача; не более трёх карточек в старом
примере; оба конца каждого ребра есть в `graph.nodes`; все `evidence_ids` разрешаются;
варианты DEMO-RENEW различаются сроками 12 и 24 месяца; неизвестный срок сохранён
как `null`; ошибка содержит `request_id`. Новый `recommendations.verified.json`:
две карточки, подтверждённый апгрейд и дополнение с `review`, без продления;
старый пример на три карточки от этого не меняется.

Совместный прогон с API: `node --test tests/e2e/demo/contract.test.mjs tests/e2e/demo/api.test.mjs`
— 11 из 11, но `api.test.mjs` покупает `DEMO-BASE` и варианты продления. Повторный
запуск без сброса демо-БД падает на ожидании `renewal`: сначала остановите API,
пересоздайте `recsys`, примените миграции и seed, затем поднимите API снова.

### Браузер

Проверено во встроенном браузере на `http://localhost:5173/#/demo` (раздаётся Vite):

- baseline (B2B, mock, авто): 3 карточки, `request_id: demo-full-1`, подпись
  «В метрику входят первые три карточки»;
- «Сравнить варианты»: `DEMO-RENEW-12 · 12 мес.`, `DEMO-RENEW-24 · 24 мес.`,
  надпись «Срок окончания неизвестен»;
- «Почему рекомендуем»: только связанное ребро `item:DEMO-BASE → item:DEMO-RENEW-12`;
- главная кнопка «Показать граф»: модальное окно с SVG и списком рёбер только этой
  карточки, без `DEMO-UPGRADE`/`DEMO-COMPLEMENT`; закрывается кнопкой;
- профиль `demo-cold (без истории)`: 2 карточки, бейджи «подтверждено» и
  «требует проверки», в тексте нет слов о прошлой покупке, продления нет;
- блок качества: таблица A/B × все/B2B/B2C (11,97% и 14,53%), под ней 18 полос
  сравнения A и B, варианты «42 из 121 (34,71%)» и «27 из 75 (36%)», отдельная
  строка «Около 55% визитов»,
  дата фида `2026-11-27`, оговорка об отсутствии исторического снимка каталога
  и baseline до `31.08.2026`; 55% не попадает в таблицу Precision@3;
- частичная выдача: баннер `insufficient_eligible_products`, 1 карточка, `demo-1`;
- пустая: «Нет рекомендаций для этого профиля.», 0 карточек;
- ошибка: `Ошибка model_unavailable (request_id: demo-error-1)`, 0 карточек;
- медленный ответ + смена профиля: через 1,9 с остаётся выдача B2C (`demo-1`),
  запоздалый ответ B2B не применён; «Выйти» очищает данные;
- HTTP-транспорт через `/api/v1` при запущенном Go API: показана 1 карточка B2C,
  консоль без ошибок.

API-проверки на запущенном бэкенде: `tests/e2e/demo/api.test.mjs`
(`node --test tests/e2e/demo/api.test.mjs`) — 7 проверок: формат выдачи, валидация
запросов и `401`, изоляция профилей по `client_id`, фидбек, уникальность `run_id`
при параллельном создании, 30 параллельных запросов. Глубокий прогон дополнительно
проверил 72 сценария (OpenAPI-поля, авторизация/регистрация, методы 405, CORS,
injection-строки, 200 смешанных параллельных операций) — 0 провалов.

Гонка при параллельном «Новый run» устранена: создание выдачи сериализуется через
`pg_advisory_xact_lock(hashtext(client_id))`.
Целостность БД проверяет `go run ./tools/checkdb`: неверных `rank` нет, «висячих»
`offer_id` нет, неверных `event_type` нет.

Нагрузочный прогон: 100 параллельных `GET /recommendations?user_id=u1&limit=3` —
100/200, p50 ~820 мс, p95 ~990 мс.

Браузерный тест добавлен в стек ЛК: `frontend-softline/tests/e2e/demo/browser.test.mjs`
на `node:test` + `playwright-core`. Браузер ищется через `CHROME_PATH`, `PATH`
(`where`/`which`) или реестр Windows — без хардкода путей; адрес экрана — `FRONTEND_URL`.
Запуск при работающем Vite: `npm run test:e2e:demo` — **13 проверок, все PASS**:
три карточки и DEMO-RENEW, варианты 12/24 и неизвестный срок, только связанное ребро,
модальный граф только этой карточки, холодный профиль с двумя бейджами,
таблица A/B, полосы сравнения и оговорки,
частичная и пустая выдача, ошибка с `request_id`, игнорирование запоздалого ответа,
карточки на главной и странице проверки БД, HTTP-транспорт через собственный Go API
тем же клиентом (Bearer), узкий экран 390 px, консоль без ошибок. Отдельно проверены
доступные ссылки на карточках и источники подтверждённых отличий.

## Замечание по зонам

По просьбе заказчика дополнительно перенесена рабочая реализация в зону участника 4:
`cmd/api`, `internal/auth`, `internal/config`, `internal/store/postgres`,
`internal/transport/http/**`, а также `internal/contract`/`internal/domain` не
изменялись. Владельцу зоны 4 — проверить совместимость с общим `Recommender` и
OpenAPI перед финальной сборкой. Утилиты `cmd/migration`, `cmd/import`, SQL-миграция,
seed и `queries.sql` перенесены в `recsys`; временная папка `bd-for-test/` удалена.
Закрытый дамп v4 восстанавливается в отдельную чистую БД по [инструкции](../../../README.md).
