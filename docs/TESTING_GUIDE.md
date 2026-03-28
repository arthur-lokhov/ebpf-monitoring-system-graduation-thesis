# Руководство по тестированию epbf-monitoring

## Быстрый старт

### 1. Запуск зависимостей

```bash
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"
cd deployments/
docker-compose up -d
```

### 2. Проверка статуса сервисов

```bash
docker-compose ps
```

Ожидаемый вывод:
```
NAME                  IMAGE                      COMMAND                  SERVICE        CREATED         STATUS
epbf-garage           dxflrs/garage:v2.2.0       "/garage server"         garage         2 minutes ago   Up 2 minutes (healthy)
epbf-monitor-server   deployments-epbf-monitor   "/app/epbf-monitor"      epbf-monitor   2 minutes ago   Up 2 minutes
epbf-postgres         postgres:15-alpine         "docker-entrypoint.s…"   postgres       2 minutes ago   Up 2 minutes (healthy)
```

---

## Тестирование API

### Health Check

Проверка работоспособности сервера:

```bash
curl http://localhost:8080/health | python3 -m json.tool
```

Ожидаемый ответ:
```json
{
    "status": "ok",
    "timestamp": "2026-03-28T11:00:00.000000Z",
    "version": "0.1.0"
}
```

### Metrics Endpoint

Проверка Prometheus metrics:

```bash
curl http://localhost:8080/metrics
```

Ожидаемый ответ:
```
# epbf-monitoring metrics
# HELP epbf_info Epbf monitoring service info
# TYPE epbf_info gauge
epbf_info{version="0.1.0"} 1
```

---

## Тестирование плагинов

### 1. Загрузка плагина

```bash
curl -s -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d "{\"git_url\": \"file:///workspace/plugins/example-container-monitor\"}" | python3 -m json.tool
```

Ожидаемый ответ:
```json
{
    "id": "uuid-плагина",
    "name": "//workspace/plugins/example-container-monitor",
    "version": "pending",
    "git_url": "file:///workspace/plugins/example-container-monitor",
    "manifest": {
        "name": "//workspace/plugins/example-container-monitor",
        "version": "pending"
    },
    "status": "pending",
    "build_log": null,
    "error_message": null,
    "created_at": "2026-03-28T11:00:00.000000Z",
    "updated_at": "2026-03-28T11:00:00.000000Z"
}
```

### 2. Проверка статуса всех плагинов

```bash
curl -s http://localhost:8080/api/v1/plugins | python3 -m json.tool
```

### 3. Проверка статуса конкретного плагина

```bash
curl -s http://localhost:8080/api/v1/plugins/<plugin-id> | python3 -m json.tool
```

### 4. Проверка артефактов сборки

```bash
ls -lh /Users/asa/Documents/epbf-monitoring/plugins/example-container-monitor/build/
```

Ожидаемый вывод (успешная сборка):
```
total 16K
-rwxr-xr-x  1 asa staff  255 Mar 28 12:00 plugin.wasm
-rw-r--r--  1 asa staff  12K Mar 28 12:00 program.o
```

---

## Отладка

### Логи сервера

```bash
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"
docker logs epbf-monitor-server 2>&1 | tail -30
```

### Логи Garage

```bash
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"
docker logs epbf-garage 2>&1 | tail -20
```

### Логи PostgreSQL

```bash
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"
docker logs epbf-postgres 2>&1 | tail -20
```

### Статус Garage

```bash
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"
docker exec epbf-garage /garage status
```

### Статус Garage (JSON)

```bash
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"
docker exec epbf-garage /garage status --format json
```

### Список бакетов Garage

```bash
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"
docker exec epbf-garage /garage bucket list
```

---

## Управление БД

### Очистка таблицы плагинов

```bash
docker exec -i epbf-postgres psql -U epbf -d epbf -c "DELETE FROM plugins;"
```

### Просмотр всех плагинов

```bash
docker exec -i epbf-postgres psql -U epbf -d epbf -c "SELECT id, name, version, status FROM plugins;"
```

### Просмотр конкретного плагина

```bash
docker exec -i epbf-postgres psql -U epbf -d epbf -c "SELECT * FROM plugins WHERE id = '<plugin-id>';"
```

---

## Полный цикл тестирования

### Сценарий 1: Успешная сборка плагина

```bash
#!/bin/bash
set -e

export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"

echo "=== 1. Очистка БД ==="
docker exec -i epbf-postgres psql -U epbf -d epbf -c "DELETE FROM plugins;"

echo "=== 2. Очистка build directory ==="
rm -rf /Users/asa/Documents/epbf-monitoring/plugins/example-container-monitor/build

echo "=== 3. Загрузка плагина ==="
PLUGIN_JSON=$(curl -s -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d "{\"git_url\": \"file:///workspace/plugins/example-container-monitor\"}")

PLUGIN_ID=$(echo "$PLUGIN_JSON" | python3 -c "import sys, json; print(json.load(sys.stdin)['id'])")
echo "Plugin ID: $PLUGIN_ID"

echo "=== 4. Ожидание сборки (60 секунд) ==="
sleep 60

echo "=== 5. Проверка статуса ==="
curl -s http://localhost:8080/api/v1/plugins/$PLUGIN_ID | python3 -m json.tool

echo "=== 6. Проверка артефактов ==="
ls -lh /Users/asa/Documents/epbf-monitoring/plugins/example-container-monitor/build/ || echo "Build dir doesn't exist"

echo "=== 7. Логи последней ошибки ==="
curl -s http://localhost:8080/api/v1/plugins/$PLUGIN_ID | python3 -c "import sys, json; data = json.load(sys.stdin)['data']; print('Status:', data['status']); print('Error:', data.get('error_message', 'None'))"
```

### Сценарий 2: Пересборка плагина

```bash
curl -s -X POST http://localhost:8080/api/v1/plugins/<plugin-id>/rebuild | python3 -m json.tool
```

### Сценарий 3: Удаление плагина

```bash
curl -s -X DELETE http://localhost:8080/api/v1/plugins/<plugin-id>
```

---

## Диагностика проблем

### Проблема: Garage не запускается

**Симптомы:** Container restarting, ошибка "No such file or directory"

**Решение:**
```bash
# Проверить права на директорию
ls -la deployments/garage/data/

# Пересоздать директорию
rm -rf deployments/garage/data
mkdir -p deployments/garage/data/meta
mkdir -p deployments/garage/data/data

# Перезапустить Garage
docker-compose restart garage
```

### Проблема: S3 connection refused

**Симптомы:** Ошибка "dial tcp 127.0.0.1:3900: connect: connection refused"

**Решение:**
```bash
# Проверить что Garage работает
docker-compose ps garage

# Проверить логи Garage
docker logs epbf-garage 2>&1 | tail -20

# Проверить что bucket существует
docker exec epbf-garage /garage bucket list
```

### Проблема: Build не работает на macOS

**Симптомы:** Ошибка "Operation not permitted" при записи в bind mount

**Решение:**
- Сборка использует docker cp вместо bind mount для output
- Убедиться что директория build существует:
  ```bash
  mkdir -p plugins/example-container-monitor/build
  ```

### Проблема: Нет credentials для S3

**Симптомы:** Ошибка "no EC2 IMDS role found"

**Решение:**
- Проверить что S3 client использует статические credentials
- Для Garage credentials могут быть пустыми

---

## Переменные окружения

| Переменная | Описание | По умолчанию |
|------------|----------|--------------|
| `PORT` | Порт HTTP сервера | `8080` |
| `DB_HOST` | Хост PostgreSQL | `postgres` |
| `DB_PORT` | Порт PostgreSQL | `5432` |
| `DB_USER` | Пользователь PostgreSQL | `epbf` |
| `DB_PASSWORD` | Пароль PostgreSQL | `epbf_password` |
| `DB_NAME` | Имя базы данных | `epbf` |
| `S3_ENDPOINT` | S3 endpoint | `http://127.0.0.1:3900` |
| `S3_REGION` | S3 регион | `garage` |
| `S3_BUCKET` | S3 бакет | `epbf-plugins` |
| `ENABLE_DOCKER` | Включить Docker для сборки | `true` |
| `BUILD_DIR` | Директория для сборки | `/tmp/epbf-builds` |

---

## Примеры успешного вывода

### Успешная загрузка плагина

```json
{
    "id": "uuid",
    "name": "example-container-monitor",
    "version": "1.0.0",
    "status": "ready",
    "ebpf_s3_key": "plugins/uuid/ebpf.o",
    "wasm_s3_key": "plugins/uuid/plugin.wasm",
    "build_log": "🔨 Building plugin...\n✅ eBPF: ...\n✅ WASM: ...\n✅ Build complete!",
    "error_message": null
}
```

### Успешная сборка (логи)

```
🔨 Building plugin...
📦 Building eBPF program...
✅ eBPF: /tmp/epbf-build-output/program.o
📦 Building WASM module...
✅ WASM: /tmp/epbf-build-output/plugin.wasm
📊 Build artifacts:
total 16K
-rwxr-xr-x    1 builder  builder      255 plugin.wasm
-rw-r--r--    1 builder  builder    11.8K program.o
✅ Build complete!
```
