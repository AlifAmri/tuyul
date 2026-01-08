# Deployment Guide

This guide describes how to deploy the TUYUL Backend application.

## Prerequisites

- Docker and Docker Compose
- Indodax API Key and Secret (for live trading)
- Redis (included in Docker Compose)

## Configuration

The application is configured via environment variables. Copy `.env.example` to `.env` and fill in the values:

```bash
cp .env.example .env
```

### Key Environment Variables

- `SERVER_PORT`: Port to run the server on (default: 8080)
- `SERVER_ENV`: Environment (development, production)
- `JWT_SECRET`: Secret for signing JWT tokens (required)
- `ENCRYPTION_KEY`: 32-byte key for encrypting API keys (required)
- `REDIS_HOST`: Redis host address
- `REDIS_PORT`: Redis port
- `INDODAX_WS_TOKEN`: Token for Indodax Public WebSocket (required for market data)

## Docker Deployment

### 1. Build and Start

Use Docker Compose to build and start the backend and Redis:

```bash
docker-compose up -d --build
```

### 2. Verify Deployment

Check the health endpoint:

```bash
curl http://localhost:8080/health
```

Expected response:
```json
{"redis":"connected","status":"ready","time":1704672000}
```

Check the logs:

```bash
docker logs -f tuyul-backend
```

## Manual Deployment (Go)

If you prefer to run the binary directly:

### 1. Build

```bash
cd backend
go build -o tuyul-app ./cmd/api/main.go
```

### 2. Run

```bash
./tuyul-app
```

## Monitoring & Logging

The application uses structured JSON logging by default in production. You can collect these logs using ELK stack, Grafana Loki, or similar tools.

- **Log Level**: Controlled by `LOG_LEVEL` (debug, info, warn, error)
- **Log Format**: Controlled by `LOG_FORMAT` (json, pretty)

## Monitoring Checkpoints

1. **Redis Connectivity**: Check `/health` endpoint.
2. **API Latency**: Monitor the `latency_ms` field in logs.
3. **Error Rates**: Monitor logs for `status >= 500`.
4. **Indodax Connection**: Check for "Successfully connected to Indodax Public WebSocket" in logs.
