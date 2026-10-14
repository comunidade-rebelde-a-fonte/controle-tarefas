# syntax=docker/dockerfile:1
# Imagem única de produção [plan M7, T-M7-06]: a SPA compilada é servida pelo mesmo binário
# Go que expõe /api/v1. Imagens oficiais com versão fixada [AGENTS §6.1]; Go 1.26 (DT-01).

# 1) SPA (React + Vite)
FROM node:22.23.3-bookworm AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# 2) API (binário estático, com a base de fusos embutida)
FROM golang:1.26.9-bookworm AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -tags timetzdata -ldflags="-s -w" -o /out/api ./cmd/api

# 3) Imagem final mínima, usuário não-root
FROM alpine:3.22.6
RUN addgroup -S -g 10001 app && adduser -S -D -H -u 10001 -G app app \
    && mkdir -p /data/storage && chown -R app:app /data
WORKDIR /app
COPY --from=api /out/api /app/api
COPY --from=web /src/frontend/dist /app/web
USER app
ENV APP_ENV=production \
    HTTP_PORT=8080 \
    STATIC_DIR=/app/web \
    FILE_STORAGE_DIR=/data/storage \
    MIGRATE_ON_BOOT=true \
    APP_TIMEZONE=America/Sao_Paulo
VOLUME ["/data/storage"]
EXPOSE 8080
HEALTHCHECK --interval=15s --timeout=3s --start-period=20s --retries=5 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/api/v1/health || exit 1
ENTRYPOINT ["/app/api"]
CMD ["serve"]
