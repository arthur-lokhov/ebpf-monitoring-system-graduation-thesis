# Запуск системы

## Архитектура

- **PostgreSQL** - Docker (метаданные)
- **Garage S3** - Docker (хранилище плагинов)
- **Prometheus** - Docker (сбор метрик)
- **UI** - Docker (React + nginx)
- **Backend** - Хост (eBPF требует доступа к ядру)

## Запуск

### 1. Запустить Docker сервисы

```bash
make docker-up
```

Это запустит:
- PostgreSQL (порт 5432)
- Garage S3 (порт 3900)
- Prometheus (порт 9090)
- UI (порт 3000)

### 2. Запустить бэкенд на хосте

```bash
make run-backend
```

Или вручную:

```bash
source deployments/.env
DB_HOST=localhost \
DB_PORT=5432 \
DB_USER=epbf \
DB_PASSWORD=epbf_password \
DB_NAME=epbf \
S3_ENDPOINT=http://localhost:3900 \
S3_REGION=garage \
S3_BUCKET=epbf-plugins \
S3_ACCESS_KEY=$GARAGE_ADMIN_TOKEN \
S3_SECRET_KEY=$GARAGE_SECRET_KEY \
ENABLE_DOCKER=true \
BUILD_DIR=/tmp/epbf-builds \
LOG_LEVEL=info \
go run ./cmd/epbf-monitor
```

### 3. Применить миграции

```bash
make migrate-run
```

### 4. Добавить плагин

```bash
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "file:///home/arthur/personal/ebpf-monitoring-system-graduation-thesis/plugins/example-container-monitor"}'
```

### 5. Проверить метрики

```bash
curl http://localhost:8080/api/v1/metrics | python3 -m json.tool
```

## Остановка

```bash
make docker-down
```

## UI

Откройте http://localhost:3000

## Prometheus

Откройте http://localhost:9090
