# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src/postbaby-backend

COPY postbaby-backend/go.mod postbaby-backend/go.sum ./
RUN go mod download

COPY postbaby-backend/cmd ./cmd
COPY postbaby-backend/internal ./internal

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/postbaby ./cmd/postbaby-backend

FROM alpine:3.22

RUN apk add --no-cache ca-certificates wget

WORKDIR /app

COPY --from=build /out/postbaby /app/postbaby
COPY index.html /app/public/index.html
COPY runtime-config.js /app/public/runtime-config.js
COPY favicon.ico manifest.json sw.js /app/public/
COPY css /app/public/css
COPY js /app/public/js
COPY img /app/public/img
COPY fonts /app/public/fonts

RUN mkdir -p /app/data

ENV POSTBABY_ADDR=0.0.0.0:8080 \
    POSTBABY_DB_PATH=/app/data/postbaby.db \
    POSTBABY_STATIC_DIR=/app/public \
    POSTBABY_DEPLOYMENT_MODE=selfhosted \
    POSTBABY_COOKIE_SECURE=false \
    POSTBABY_SESSION_TTL=720h

EXPOSE 8080

VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O - http://127.0.0.1:8080/api/health > /dev/null || exit 1

CMD ["/app/postbaby"]
