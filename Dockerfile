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
RUN apk add --no-cache ca-certificates tzdata
COPY --from=backend /parkpulse /usr/local/bin/parkpulse
# `pulse-cli` buyrug'i — konteynerda AI agent bilan interaktiv sessiya (claude kabi).
COPY --from=backend /pulse-cli /usr/local/bin/pulse-cli
COPY --from=frontend /build/out /app/static

ENV STATIC_DIR=/app/static \
    LISTEN_ADDR=:8888

EXPOSE 8888
ENTRYPOINT ["parkpulse"]
