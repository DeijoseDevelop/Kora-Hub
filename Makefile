# Copyright (C) 2026 Deijose <tech@deijose.dev>
# SPDX-License-Identifier: AGPL-3.0-only
# Kora Hub — Makefile
# Pipeline local: frontend -> embed -> binario único (P3).

SHELL := /bin/bash
GO      := go
GOFLAGS ?=
VERSION ?= dev
BIN     := bin/kora-hub
LDFLAGS := -s -w -X github.com/DeijoseDevelop/Kora-Hub/internal/server.Version=$(VERSION)

.PHONY: all dev build frontend generate db test vet lint run docker desktop-dev desktop-build clean

all: build

## frontend: instala deps y compila el frontend Nix.js
frontend:
	cd web && npm install
	cd web && npm run build
	rm -rf internal/web/dist && cp -r web/dist internal/web/dist

## generate: regenera el código sqlc
generate:
	sqlc generate

## build: binario único con frontend embebido
build: frontend
	mkdir -p bin
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/hub

## dev: backend en modo desarrollo (frontend servido por Vite en :5173)
dev:
	HUB_JWT_SECRET=dev-secret HUB_DB_PATH=data/hub.db $(GO) run ./cmd/hub

## test: tests unitarios e integración
test:
	$(GO) test ./...

## vet: análisis estático
vet:
	$(GO) vet ./...
	cd web && npx tsc --noEmit

## docker: imagen multi-stage con binario final
docker:
	docker build -t kora-hub:$(VERSION) .

## desktop-dev: sidecar + shell Tauri en modo desarrollo
desktop-dev:
	cd desktop && npm install
	cd desktop && npm run build:sidecar
	cd desktop && npm run dev

## desktop-build: instalables nativos (.deb/.AppImage/.msi/.exe/.dmg)
desktop-build:
	cd desktop && npm install
	cd desktop && npm run build

## run: ejecuta el binario local
run: build
	HUB_JWT_SECRET=dev-secret HUB_DB_PATH=data/hub.db ./$(BIN)

clean:
	rm -rf bin dist web/dist internal/web/dist data
