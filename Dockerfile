# Copyright (C) 2026 Deijose <tech@deijose.dev>
# SPDX-License-Identifier: AGPL-3.0-only
# Kora Hub — un solo contenedor, cero dependencias externas (P3).
# Multi-stage: frontend Nix.js -> binario Go estático -> scratch.

# ---- Stage 1: frontend Nix.js ----
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- Stage 2: backend Go ----
FROM golang:1.26-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 go build -ldflags "-s -w" -o /out/kora-hub ./cmd/hub

# ---- Stage 3: imagen final ----
FROM scratch
COPY --from=backend /out/kora-hub /kora-hub
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
VOLUME ["/data"]
EXPOSE 8080
ENV HUB_BIND=:8080 \
    HUB_DB_PATH=/data/hub.db \
    HUB_DATA_DIR=/data
ENTRYPOINT ["/kora-hub"]
