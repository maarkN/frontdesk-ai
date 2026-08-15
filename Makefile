# FrontDesk AI — atalhos de desenvolvimento.
#   make up        sobe a stack local (observabilidade sempre ligada)
#   make up PROFILES=--profile asterisk   inclui o media backend próprio
#   make test      go + python + typescript

COMPOSE  ?= docker compose
PROFILES ?=

.PHONY: up down logs build test test-go test-py test-ts fmt

up:
	$(COMPOSE) $(PROFILES) up -d --build

down:
	$(COMPOSE) $(PROFILES) down --remove-orphans

logs:
	$(COMPOSE) logs -f --tail=100

build:
	$(COMPOSE) $(PROFILES) build

test: test-go test-py test-ts

test-go:
	cd go && go test ./...

test-py:
	cd python/agent-runtime && uv run pytest

test-ts:
	pnpm -r typecheck

fmt:
	cd go && gofmt -w ./cmd ./internal
	cd python/agent-runtime && uv run ruff format && uv run ruff check --fix
