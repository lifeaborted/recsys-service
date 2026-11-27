# Сторонние компоненты

Версии Go-модулей взяты из `go.mod`/`go.sum`, версии фронтенда — из `frontend-softline/package-lock.json`. Полные тексты лицензий здесь не копируются, только ссылки. Ключи, DSN и закрытые данные в файл не добавлены.

## Обязательные компоненты стенда

| Компонент | Версия | Лицензия | Ссылка | Зачем |
|---|---|---|---|---|
| PostgreSQL | 17 (тег `pg17`) | PostgreSQL License | https://www.postgresql.org/about/licence/ | Основная БД: каталог, продажи, снимки, сессии, доказательства и рекомендации. |
| Образ `pgvector/pgvector` | тег `pg17`; версия расширения отдельно не закреплена | PostgreSQL License | https://github.com/pgvector/pgvector | PostgreSQL 17 с расширениями `pg_trgm` и `vector` для эмбеддингов и поиска. |
| `github.com/jackc/pgx/v5` | 5.13.0 | MIT | https://github.com/jackc/pgx | Драйвер и пул подключений к PostgreSQL. |
| `llama.cpp` (локальный сервер) | не зафиксирована в репозитории; путь задаётся `LLAMA_SERVER` | MIT | https://github.com/ggml-org/llama.cpp | Локальный OpenAI-совместимый сервер для чата и эмбеддингов без внешнего API. |
| Qwen3-Embedding-0.6B | модель `Qwen3-Embedding-0.6B`; конкретный GGUF-файл и квантование задаются в `EMBED_MODEL` и в репозитории не закреплены | Apache-2.0 | https://huggingface.co/Qwen/Qwen3-Embedding-0.6B | Локальная модель эмбеддингов для офлайн-прогона каталога и семантического поиска. |

## Go-зависимости

| Компонент | Версия | Лицензия | Ссылка | Зачем |
|---|---|---|---|---|
| `google.golang.org/grpc` | 1.85.0 | Apache-2.0 | https://github.com/grpc/grpc-go | gRPC-сервер рекомендаций и клиент HTTP API. |
| `google.golang.org/protobuf` | 1.36.12 | BSD-3-Clause | https://github.com/protocolbuffers/protobuf-go | Go-представление контракта и сериализация сообщений. |
| `github.com/lib/pq` | 1.10.9 | MIT | https://github.com/lib/pq | Дополнительный PostgreSQL-драйвер, используется утилитами и тестами. |
| `golang.org/x/crypto` | 0.59.0 | BSD-3-Clause | https://pkg.go.dev/golang.org/x/crypto | bcrypt для паролей демо-аккаунтов. |
| `golang.org/x/sync` | 0.25.0 | BSD-3-Clause | https://pkg.go.dev/golang.org/x/sync | Вспомогательные примитивы синхронизации (косвенно, через pgx). |
| `golang.org/x/text` | 0.44.0 | BSD-3-Clause | https://pkg.go.dev/golang.org/x/text | Текстовые преобразования (косвенно, через pgx). |

## Фронтенд

| Компонент | Версия | Лицензия | Ссылка | Зачем |
|---|---|---|---|---|
| React | 18.3.1 | MIT | https://react.dev/ | UI демо-экрана и ЛК. |
| React DOM | 18.3.1 | MIT | https://react.dev/ | Рендеринг React в браузере. |
| Vite | 6.4.6 | MIT | https://vite.dev/ | Dev-сервер и сборка фронтенда. |
| TypeScript | 5.7.3 | Apache-2.0 | https://www.typescriptlang.org/ | Проверка типов. |
| `@vitejs/plugin-react` | 4.7.0 | MIT | https://github.com/vitejs/vite-plugin-react | React-плагин Vite. |
| `playwright-core` | 1.65.1 | Apache-2.0 | https://playwright.dev/ | Браузерный E2E демо-экрана на системном Chrome/Edge. |
| `@types/react` | 18.3.32 | MIT | https://www.npmjs.com/package/@types/react | Типы React. |
| `@types/react-dom` | 18.3.7 | MIT | https://www.npmjs.com/package/@types/react-dom | Типы React DOM. |

## Что не включено

- Чат-модель отдельной строкой не указана: в `recsys/.env.example` поле `LLM_MODEL=` пустое, конкретная модель и версия в открытом репозитории не зафиксированы. Если она появится в локальном `.env`, её имя и лицензию нужно добавить сюда.
- Внешние платные API рекомендаций/эмбеддингов не используются: базовый сценарий работает с локальным `llama.cpp`.
