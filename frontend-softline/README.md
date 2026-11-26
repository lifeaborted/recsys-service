# Softline LC — клон лчного кабинета на React + Vite + TypeScript

Локальный клон `lk.softline.ru` для проверки базы данных и рекомендательного
алгоритма. Визуальная часть взята из присланного снимка: оригинальный CSS,
шрифты, картинки и разметка шапки/сайдбара/виджетов лежат в `public/assets/` и
`src/data/chrome.ts`. Вся логика переписана под текущий стек проекта — React 18,
Vite 6, TypeScript. Реальные логины/пароли не используются и никуда не отправляются.

## Запуск

```bash
npm install
npm run dev        # http://localhost:5173
```

Проверка типов и production-сборка:

```bash
npm run typecheck
npm run build
npm run preview    # http://localhost:4173
```

По умолчанию включён **mock-режим** — сервер БД не нужен.

## Что где лежит

```
index.html                 подключение исходного CSS и точки входа React
public/assets/             CSS, шрифты и картинки lk.softline.ru
tools/gen-fragments.mjs    пересоздание разметки из E:\Главная.html
src/data/chrome.ts         сгенерированные HEADER/SIDEBAR/виджеты (не редактировать)
src/data/catalog.ts        каталог Softline (продукты, цены, картинки)
src/data/account.ts        пользователи, заказы, счета, ключи, уведомления
src/api/                   клиент recommend/feedback/health + mock-реализация
src/hooks/useRecommendations.ts     загрузка выдачи, отмена, метрики времени
src/hooks/useCloneRecommendations.ts  связка с панелью отладки и фидбеком
src/hooks/useCloneUi.ts     слайдеры, меню, алерты статической разметки
src/state/settings.tsx     userId, scenario, limit, baseUrl, token, mockMode
src/state/debug.tsx        снимки запросов/ответов, история, score-режим
src/components/            оболочка, карточки и слайдер рекомендаций, debug-панель
src/pages/                 Главная, Тест рекомендаций, Каталог, Счета, Ключи,
                           Уведомления, Заказы, Профиль и заглушки разделов
src/styles/clone.css       дополнения к исходному CSS
```

## Проверка БД / алгоритма

1. Откройте **Профиль** (`#/profile`): выберите пользователя, сценарий, лимит,
   при необходимости выключите Mock и укажите базовый URL API и токен.
   Кнопкой «Проверить соединение» вызывается `GET /health`.
2. Откройте **Тест рекомендаций** (`#/recommendations`) или главную — блок
   «Вам может подойти» строится по ответу вашей модели.
3. Плавающая кнопка **«⚙ Тест рекомендаций»** показывает: запрос, ответ, ошибку,
   таблицу score/причин, режим (mock/live), время отклика и историю вызовов.
   Кнопка «Score» включает оценку и объяснение прямо на карточках.
4. Клик по карточке отправляет `POST /feedback` (в mock — просто в консоль).

### Контракт API

Vite проксирует `/api` на `VITE_PROXY_TARGET` (по умолчанию `http://localhost:8000`),
поэтому CORS настраивать не нужно.

| Метод | Путь | Назначение |
|-------|------|------------|
| GET | `/recommendations?user_id=&limit=&scenario=&cursor=` | выдача |
| POST | `/feedback` | обратная связь |
| GET | `/health` | проверка соединения |

Ответ рекомендаций (терпим к `items`, `recommendations` или массиву в корне):

```json
{
  "user_id": "u1",
  "generated_at": "2026-11-26T10:00:00.000Z",
  "model_version": "ranker-1.4.0",
  "request_id": "req-7f3a",
  "items": [
    {
      "item_id": "p001",
      "title": "Kaspersky Security для почтовых серверов. Базовая",
      "subtitle": "«Лаборатория Касперского»",
      "category": "Безопасность",
      "score": 0.93,
      "reason": "Часто подключают вместе",
      "price": 1512,
      "currency": "RUB",
      "tags": ["антивирус", "сервер"],
      "image_url": "/assets/scale-dec-168x168.png",
      "url": "https://store.softline.ru/kaspersky/kl4313rarfs-3851/"
    }
  ]
}
```

`item_id` и `title` — обязательны, остальное подставляется из локального каталога.
Сценарии: `demo` (эталонный порядок lk.softline.ru), `upsell`, `cross-sell`,
`onboarding`, `retention`.

### Пересоздание вёрстки из снимка

Если исходный файл снова лежит на `E:\` (`Главная.html` и `Главная_files`),
выполните из корня:

```bash
node tools/gen-fragments.mjs
```

Скрипт перезаписывает только `src/data/chrome.ts`. Свои правки делайте в
`src/pages`, `src/components`, `src/hooks`, `src/api`, `src/data/catalog.ts`,
`src/data/account.ts` и `src/styles/clone.css`.

## Ограничения

- Данные каталога, заказов и активности статические — нужны только для наполнения UI.
- Авторизации нет: это инструмент локальной проверки интеграции, не продакшен-кабинет.
- Токен хранится в `localStorage` браузера; не используйте реальные секреты на публичном стенде.
