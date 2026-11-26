GO ?= go
NODE ?= node
PWSH ?= pwsh
COMPOSE ?= docker compose
DEV_COMPOSE = $(COMPOSE) -p recsys-dev -f compose.yml
API_URL ?= http://127.0.0.1:8080
ARGS ?=

.PHONY: up down run seed import enrich build-signals backtest swagger check test-e2e test-unit

up:
	$(DEV_COMPOSE) up -d --wait postgres

down:
	$(DEV_COMPOSE) down

run:
	$(GO) run -mod=readonly ./cmd/api

seed:
	$(GO) run -mod=readonly ./cmd/seed -migrate $(ARGS)

import:
	$(GO) run -mod=readonly ./cmd/import $(ARGS)

enrich:
	$(GO) run -mod=readonly ./cmd/enrich $(ARGS)

build-signals:
	$(GO) run -mod=readonly ./cmd/build-signals $(ARGS)

backtest:
	$(GO) run -mod=readonly ./cmd/backtest $(ARGS)

swagger:
	curl --fail --silent --show-error $(API_URL)/swagger/openapi.yaml

check:
	$(GO) build -mod=readonly ./...
	$(GO) vet -mod=readonly ./...
	$(MAKE) test-unit
	$(NODE) --test tests/e2e/demo/contract.test.mjs

test-unit:
	$(GO) test -mod=readonly -count=1 ./...

test-e2e:
	$(PWSH) -NoProfile -File tests/e2e/platform/check.ps1
