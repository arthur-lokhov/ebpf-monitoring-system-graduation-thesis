# Известные проблемы и решения

## 🚨 Критичная проблема: macOS Virtualization Framework

### Проблема
При запуске на macOS с Docker Desktop / Colima возникает ошибка:
```
error: unable to open output file '/workspace/plugin/build/program.o': 'Operation not permitted'
```

### Причина
macOS Virtualization Framework имеет ограничения на запись в bind mount директории из контейнеров. Это известная проблема безопасности macOS.

### Решение 1: Использовать Linux
Запускать на Linux хосте где bind mounts работают корректно.

### Решение 2: Использовать docker cp
Вместо bind mount для output directory использовать `docker cp` для копирования артефактов из контейнера.

### Решение 3: Использовать Linux VM
Запускать в полноценной Linux VM (не Docker Desktop):
```bash
# На Linux VM
docker-compose up -d
```

## ✅ Рабочий функционал

### 1. API Server
- ✅ Health endpoint
- ✅ Metrics endpoint (Prometheus format)
- ✅ Plugins CRUD API
- ✅ PostgreSQL интеграция
- ✅ S3 (Garage) интеграция

### 2. Plugin Loading
- ✅ Загрузка из Git репозиториев
- ✅ Парсинг manifest.yml
- ✅ Валидация структуры

### 3. Plugin Building
- ✅ Сборка в Docker контейнере
- ✅ eBPF компиляция (clang)
- ✅ WASM компиляция (clang wasm32)
- ✅ Логирование сборки
- ✅ Обработка ошибок

### 4. Storage
- ✅ Сохранение метаданных в PostgreSQL
- ✅ Сохранение build logs
- ✅ S3 для eBPF/WASM артефактов

## ⚠️ Требует доработки

### 1. WASM Runtime
Не реализован запуск WASM модулей:
```go
// TODO: WASM Runtime
// - wasmtime-go интеграция
// - Host functions для плагинов
// - Event loop
```

### 2. eBPF Loader
Не реализована загрузка eBPF в ядро:
```go
// TODO: eBPF Loader
// - cilium/ebpf интеграция
// - Program loading
// - Map management
```

### 3. Metrics Collector
Не реализован сбор метрик:
```go
// TODO: Metrics Collector
// - Сбор от WASM плагинов
// - Агрегация
// - Экспорт в Prometheus
```

### 4. Filter Engine
Не реализована PromQL фильтрация:
```go
// TODO: Filter Engine
// - PromQL парсер
// - Функции (rate, sum, avg)
// - Executor
```

### 5. UI
Не реализован веб интерфейс:
```go
// TODO: UI
// - React + shadcn/ui
// - Plugin Manager
// - Metrics Browser
// - Dashboard (Grafana Scenes)
```

## 📝 План доработок

### Приоритет 1 (Критично)
1. Исправить bind mount проблему на macOS (использовать docker cp)
2. Реализовать WASM Runtime
3. Реализовать eBPF Loader

### Приоритет 2 (Важно)
4. Metrics Collector
5. Filter Engine

### Приоритет 3 (Желательно)
6. UI
7. Grafana интеграция

## 🔧 Временное решение для тестирования

Для тестирования на macOS можно использовать:

1. **Собрать плагин вручную:**
```bash
cd plugins/example-container-monitor
./build-docker.sh
```

2. **Загрузить артефакты напрямую в S3:**
```bash
# Через mc (MinIO Client)
mc cp build/program.o myminio/epbf-plugins/<plugin-id>/ebpf.o
mc cp build/plugin.wasm myminio/epbf-plugins/<plugin-id>/plugin.wasm
```

3. **Обновить статус в БД:**
```sql
UPDATE plugins 
SET status = 'ready', 
    ebpf_s3_key = '<plugin-id>/ebpf.o',
    wasm_s3_key = '<plugin-id>/plugin.wasm'
WHERE id = '<plugin-id>';
```

## 📊 Статус проекта

| Компонент | Статус | Готовность |
|-----------|--------|------------|
| API Server | ✅ Работает | 100% |
| PostgreSQL | ✅ Работает | 100% |
| S3 Storage | ✅ Работает | 100% |
| Plugin Loader | ✅ Работает | 100% |
| Plugin Builder | ⚠️ Проблема с macOS | 80% |
| WASM Runtime | ❌ Не реализован | 0% |
| eBPF Loader | ❌ Не реализован | 0% |
| Metrics Collector | ❌ Не реализован | 0% |
| Filter Engine | ❌ Не реализован | 0% |
| UI | ❌ Не реализован | 0% |

**Общая готовность MVP:** ~40%
