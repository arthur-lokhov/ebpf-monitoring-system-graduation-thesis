# Тестирование epbf-monitoring

## Быстрый старт

### 1. Запуск зависимостей

```bash
cd deployments/
docker-compose up -d
```

Проверка:
```bash
docker-compose ps
# epbf-postgres   Up (healthy)
# epbf-garage     Up (healthy)
```

### 2. Инициализация Garage S3

```bash
./init-garage.sh
```

Или вручную:
```bash
# Получить node ID
docker exec epbf-garage /garage node id

# Assign layout (замените <node_id> на ваш)
docker exec epbf-garage /garage layout assign <node_id> -z default -c 10G
docker exec epbf-garage /garage layout apply --version 1

# Создать bucket
docker exec epbf-garage /garage bucket create epbf-plugins

# Создать ключ
docker exec epbf-garage /garage key create epbf-admin
docker exec epbf-garage /garage key allow <key_id> --create-bucket
```

### 3. Запуск сервера

```bash
cd ..
go run ./cmd/epbf-monitor
```

Или используйте бинарник:
```bash
./epbf-monitor
```

Проверка:
```bash
curl http://localhost:8080/health
# {"status":"ok","timestamp":"..."}

curl http://localhost:8080/metrics
# epbf_info{version="0.1.0"} 1
```

---

## Тестирование плагинов

### Вариант 1: Тестирование example плагина

#### Сборка плагина

```bash
cd plugins/example-container-monitor/
./build.sh
```

Ожидаемый вывод:
```
🔨 Building container-monitor plugin...
📦 Building eBPF program...
✅ eBPF: build/program.o
📦 Building WASM module...
✅ WASM: build/plugin.wasm
```

#### Проверка артефактов

```bash
ls -lh build/
# program.o   (eBPF объект, ~10-50KB)
# plugin.wasm (WASM модуль, ~5-20KB)

# Проверка eBPF
file build/program.o
# ELF 64-bit LSB relocatable, eBPF

# Проверка WASM
file build/plugin.wasm
# WebAssembly (wasm) binary module
```

#### Загрузка плагина через API

```bash
# Добавить плагин
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d "{
    \"git_url\": \"file://$(pwd)\"
  }"

# Проверить статус
curl http://localhost:8080/api/v1/plugins

# Детали плагина
curl http://localhost:8080/api/v1/plugins/<plugin_id>
```

### Вариант 2: Тестирование с Git репозиторием

```bash
# Создайте репозиторий с плагином
git init my-plugin
cd my-plugin

# Скопируйте файлы плагина
cp -r ../plugins/example-container-monitor/* .
git add .
git commit -m "Initial commit"

# Загрузите через API
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{
    "git_url": "file:///path/to/my-plugin"
  }'
```

---

## Проверка работы

### 1. Health check

```bash
curl http://localhost:8080/health | jq
```

Ожидаемый ответ:
```json
{
  "status": "ok",
  "timestamp": "2024-03-26T14:00:00Z",
  "version": "0.1.0"
}
```

### 2. Metrics endpoint

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

### 3. Plugins API

```bash
# Список плагинов
curl http://localhost:8080/api/v1/plugins | jq

# Добавить плагин
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "https://github.com/example/plugin.git"}' | jq

# Получить плагин
curl http://localhost:8080/api/v1/plugins/<id> | jq

# Пересобрать плагин
curl -X POST http://localhost:8080/api/v1/plugins/<id>/rebuild | jq
```

### 4. Проверка логов

```bash
# Логи сервера
journalctl -u epbf-monitor -f

# Или при запуске через go run
# (логи выводятся в stdout)
```

---

## Интеграционные тесты

### Тест 1: Полный цикл загрузки плагина

```bash
#!/bin/bash
set -e

echo "1. Запуск сервера..."
go run ./cmd/epbf-monitor &
SERVER_PID=$!
sleep 3

echo "2. Проверка health..."
curl -f http://localhost:8080/health

echo "3. Загрузка плагина..."
PLUGIN_ID=$(curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "file://'"$(pwd)"'/plugins/example-container-monitor"}' \
  | jq -r '.id')

echo "4. Ожидание сборки..."
sleep 10

echo "5. Проверка статуса..."
STATUS=$(curl http://localhost:8080/api/v1/plugins/$PLUGIN_ID | jq -r '.status')
if [ "$STATUS" = "ready" ]; then
    echo "✅ Тест пройден"
else
    echo "❌ Тест провален: статус=$STATUS"
    kill $SERVER_PID
    exit 1
fi

echo "6. Очистка..."
kill $SERVER_PID
```

### Тест 2: Проверка PostgreSQL

```bash
# Подключение к БД
docker exec -it epbf-postgres psql -U epbf -d epbf

# Проверка таблиц
\dt

# Проверка плагинов
SELECT id, name, version, status FROM plugins;
```

### Тест 3: Проверка S3 (Garage)

```bash
# Проверка бакета
docker exec epbf-garage /garage bucket list

# Проверка ключей
docker exec epbf-garage /garage key list
```

---

## Отладка

### Проблемы сборки плагинов

```bash
# Проверка логов сборки
curl http://localhost:8080/api/v1/plugins/<id> | jq '.build_log'

# Локальная сборка
cd plugins/example-container-monitor/
./build.sh 2>&1
```

### Проблемы с eBPF

```bash
# Проверка версии ядра
uname -r
# Должно быть >= 5.8

# Проверка доступности eBPF
bpftool prog list

# Проверка загруженных программ
bpftool prog show
```

### Проблемы с WASM

```bash
# Проверка экспортов
wasm-objdump -x plugins/example-container-monitor/build/plugin.wasm

# Должно быть:
# - epbf_init
# - __data_end
# - __heap_base
```

---

## Производительность

### Benchmark: Загрузка плагина

```bash
time curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "file://..."}'
```

### Benchmark: Обработка событий

Запустите тестовую нагрузку и измерьте:
- Задержку обработки событий
- Использование памяти
- CPU usage

---

## Очистка

```bash
# Остановить сервер
pkill epbf-monitor

# Остановить Docker
cd deployments/
docker-compose down

# Очистить данные
docker-compose down -v
```
