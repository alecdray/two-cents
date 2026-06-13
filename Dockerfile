# CSS build stage
FROM node:22-bookworm-slim AS css-builder

WORKDIR /build

COPY package.json package-lock.json* ./
RUN npm install

# Tailwind v4 scans these sources for class usage
COPY static/src/ ./static/src/
COPY src/ ./src/

RUN npx @tailwindcss/cli -i ./static/src/main.css -o ./static/public/main.css

# Go build stage
FROM golang:1.26-bookworm AS builder

WORKDIR /build

# Build dependencies for go-sqlite3 (requires CGO)
RUN apt-get update && apt-get install -y gcc libc6-dev && rm -rf /var/lib/apt/lists/*

# Download dependencies first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Install templ code generator
RUN go install github.com/a-h/templ/cmd/templ@v0.3.977

# Copy source, generate templates, build
COPY src/ ./src/
RUN templ generate ./src/
RUN CGO_ENABLED=1 GOOS=linux go build -v -o ./bin/app ./src/cmd/app.go

# Runtime stage
FROM debian:bookworm-slim

WORKDIR /app

# SQLite runtime library (needed by the go-sqlite3 CGO binary)
RUN apt-get update && apt-get install -y libsqlite3-0 ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /build/bin/app ./bin/app
COPY static/public/ ./static/public/
COPY --from=css-builder /build/static/public/main.css ./static/public/main.css
COPY db/migrations/ ./db/migrations/

# Data directory for SQLite (mount a volume here — holds db + Teller cert)
RUN mkdir -p /data

EXPOSE 4690

ENV ENV=production
ENV PORT=4690
ENV DB_PATH=/data/db.sql
ENV GOOSE_DRIVER=sqlite3
ENV GOOSE_DBSTRING=/data/db.sql
ENV GOOSE_MIGRATION_DIR=./db/migrations

ENTRYPOINT ["./bin/app"]
