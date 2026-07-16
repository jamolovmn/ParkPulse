# ---------- 1-bosqich: Frontend (Next.js -> static out/) ----------
FROM node:20-alpine AS frontend
WORKDIR /build
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# ---------- 2-bosqich: Backend (Golang -> bitta binary) ----------
FROM golang:1.25-alpine AS backend
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
# Server (entrypoint = parkpulse) + interaktiv agent CLI (pulse-cli buyrug'i).
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /parkpulse ./cmd/server \
 && CGO_ENABLED=0 go build -ldflags="-s -w" -o /pulse-cli ./cmd/parkpulse-cli

# ---------- 3-bosqich: Yakuniy yengil image ----------
FROM alpine:3.20
# sudo — agent (bash tool) root huquqli buyruqlar bajarishi uchun. Konteyner
# odatda root sifatida ishlaydi (sudo parolsiz), non-root bo'lsa UI'dagi sudo
# paroli askpass orqali ishlatiladi.
RUN apk add --no-cache ca-certificates tzdata sudo
COPY --from=backend /parkpulse /usr/local/bin/parkpulse
# `pulse-cli` buyrug'i — konteynerda AI agent bilan interaktiv sessiya (claude kabi).
COPY --from=backend /pulse-cli /usr/local/bin/pulse-cli
COPY --from=frontend /build/out /app/static
# Entrypoint: host'ga `pulse`/`parkpulse` buyruqlarini avto o'rnatadi (bin mount bo'lsa),
# so'ng serverni ishga tushiradi.
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod +x /usr/local/bin/docker-entrypoint.sh

ENV STATIC_DIR=/app/static \
    LISTEN_ADDR=:8888 \
    PULSE_HOST_BIN=/host/bin

EXPOSE 8888
ENTRYPOINT ["docker-entrypoint.sh"]
