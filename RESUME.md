# Резюме: Проект eBPF Monitoring Platform

## 📋 Описание проекта

**eBPF Monitoring Platform** — модульная платформа мониторинга инфраструктуры на основе технологии eBPF с поддержкой пользовательских плагинов, распространяемых через Git-репозитории.

**Стек технологий:**
- **Backend:** Go 1.25, PostgreSQL, Garage S3
- **eBPF:** C, libbpf, cilium/ebpf
- **WASM:** wasmtime-go, clang wasm32
- **Metrics:** Prometheus Go client, динамические метрики
- **Infrastructure:** Docker, Docker Compose, Colima/WSL2

---

## 🏆 Ключевые достижения

### 1. Архитектура и дизайн системы
- **Спроектировал модульную архитектуру** с разделением ответственности между компонентами (Plugin Service, Runtime, Metrics Collector)
- **Реализовал плагин-ориентированный дизайн** — плагины распространяются как Git-репозитории с manifest.yml
- **Разработал систему динамических метрик** — метрики регистрируются на лету из manifest.yml плагина

### 2. eBPF интеграция
- **Интегрировал eBPF программы** через cilium/ebpf для мониторинга на уровне ядра Linux
- **Реализовал загрузку eBPF** из S3 с верификацией и обработкой ошибок
- **Настроил ring buffer** для получения событий от eBPF программ в реальном времени
- **Обработал ограничения Docker** (memlock, capabilities) для работы eBPF в контейнерах

### 3. WASM Runtime
- **Интегрировал wasmtime-go** для безопасного выполнения пользовательского кода
- **Реализовал host functions** для взаимодействия WASM с платформой (логирование, эммит метрик)
- **Настроил CGO** для компиляции wasmtime-go в Docker (gcc, musl-dev, bookworm)
- **Обеспечил изоляцию** — WASM плагины выполняются в песочнице без доступа к хосту

### 4. Система метрик Prometheus
- **Разработал динамическую регистрацию метрик** — метрики создаются из manifest.yml плагина
- **Поддержал все типы метрик:** Counter, Gauge, Histogram
- **Реализовал обработку labels** — парсинг YAML, type assertions ([]any, []string)
- **Настроил Prometheus endpoint** `/metrics` с правильным форматом экспорта

### 5. Plugin Lifecycle Management
- **Реализовал полный цикл жизни плагина:**
  - Git clone из репозитория
  - Docker-based сборка (eBPF + WASM)
  - Загрузка артефактов в S3
  - Регистрация метрик
  - Запуск runtime (eBPF + WASM)
  - Enable/Disable/Delete операции
- **Настроил асинхронную сборку** — плагины собираются в фоне без блокировки API
- **Реализовал логирование сборки** — полный build log сохраняется в БД

### 6. Инфраструктура и DevOps
- **Создал multi-stage Dockerfile** для минимизации размера образа
- **Настроил Docker Compose** с PostgreSQL, Garage S3, epbf-monitor
- **Решил проблемы совместимости:**
  - Alpine → Bookworm для CGO поддержки
  - memlock ulimits для eBPF
  - host.docker.internal для доступа к S3
- **Настроил rootless режим** — работа без root прав благодаря WASM изоляции

### 7. API и HTTP сервер
- **Разработал REST API** с chi router:
  - `POST /api/v1/plugins` — добавить плагин
  - `GET /api/v1/plugins/{id}` — получить плагин
  - `DELETE /api/v1/plugins/{id}` — удалить плагин
  - `POST /api/v1/plugins/{id}/enable` — включить
  - `POST /api/v1/plugins/{id}/disable` — выключить
  - `GET /metrics` — Prometheus экспорт
- **Реализовал middleware** — logging, CORS, recovery, timeout
- **Настроил graceful shutdown** — корректное завершение работы при SIGINT/SIGTERM

### 8. Работа с данными
- **PostgreSQL:**
  - Схема БД с миграциями
  - pgx driver с connection pooling
  - Обработка NULL полей (pgtype.Text)
- **S3 (Garage):**
  - aws-sdk-go-v2 интеграция
  - Upload/Download/Delete операции
  - Health check endpoint
  - Retry logic

### 9. Логирование и наблюдаемость
- **Внедрил zap logger** — структурированное логирование с уровнями
- **Настроил детальное логирование** — отладка парсинга manifest, labels, metrics
- **Добавил метрики платформы:**
  - `epbf_plugin_builds_total` — количество сборок
  - `epbf_ebpf_programs_loaded` — загруженные eBPF программы
  - `epbf_wasm_instances_active` — активные WASM инстансы

### 10. Решение проблем production-ready
- **Исправил парсинг YAML** — labels терялись при конвертации map[string]any
- **Решил проблему CGO в Docker** — switched from alpine to bookworm
- **Обработал memlock ограничения** — ulimits в docker-compose.yml
- **Настроил proper error handling** — BuildError type с build log

---

## 📊 Технические результаты

| Метрика | Значение |
|---------|----------|
| Строк Go кода | ~4500 |
| Строк C (eBPF+WASM) | ~500 |
| Файлов кода | 35+ |
| Git коммитов | 25+ |
| Docker образов | 2 (builder, runtime) |
| API endpoints | 10+ |
| Динамических метрик | 4 на плагин |
| Время сборки плагина | ~1 секунда |
| Размер binary | ~25 MB |

---

## 🎯 Приобретённые навыки

### Hard Skills
- **Go Programming:** Concurrency (goroutines, channels), context, interfaces, error handling
- **eBPF:** bpf programs, maps, ring buffer, cilium/ebpf library
- **WASM:** wasmtime-go, host functions, sandboxing, CGO integration
- **Docker:** Multi-stage builds, ulimits, capabilities, volume mounts
- **PostgreSQL:** Schema design, migrations, pgx driver, connection pooling
- **S3:** aws-sdk-go-v2, upload/download, error handling
- **Prometheus:** Go client, dynamic metrics, Counter/Gauge/Histogram
- **REST API:** chi router, middleware, JSON encoding, error responses
- **YAML Parsing:** gopkg.in/yaml.v3, type assertions, nested structures
- **Git:** go-git library, clone, commit parsing

### Soft Skills
- **Problem Solving:** Debugged complex issues (CGO, memlock, labels parsing)
- **System Design:** Designed modular architecture with clear boundaries
- **Documentation:** Created comprehensive README, TESTING_GUIDE, KNOWN_ISSUES
- **Testing:** Manual testing on WSL, Docker debugging, log analysis

---

## 🚀 Готовый продукт

Проект полностью функционален и готов к демонстрации:

```bash
# Запуск
cd deployments/
docker-compose up -d

# Проверка
curl http://localhost:8080/health
curl http://localhost:8080/metrics | grep epbf

# Добавление плагина
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "file:///path/to/plugin"}'
```

---

## 📝 Примеры кода для портфолио

### 1. Динамическая регистрация метрик
```go
// internal/metrics/dynamic.go
func (d *DynamicMetrics) RegisterPluginMetrics(
    pluginName string, 
    manifest map[string]any,
) error {
    metricsList := parseMetricsFromManifest(manifest)
    for _, m := range metricsList {
        switch m.Type {
        case "counter":
            collector = prometheus.NewCounterVec(...)
        case "gauge":
            collector = prometheus.NewGaugeVec(...)
        }
        d.registry.Register(collector)
    }
}
```

### 2. eBPF загрузка из S3
```go
// internal/plugin/runtime.go
func (r *Runtime) StartPlugin(...) error {
    ebpfBytes := r.downloadFromS3(ctx, ebpfS3Key)
    program, err := r.ebpfLoader.LoadProgram(
        ctx, pluginID, name, ebpfBytes,
        func(event ebpf.ContainerEvent) {
            r.metrics.EBPFEventReceived(name, event.Type)
        },
    )
}
```

### 3. WASM host functions
```go
// internal/runtime/wasm/engine.go
func (e *Engine) defineHostFunctions(linker *wasmtime.Linker) {
    linker.FuncWrap("env", "epbf_log", func(ptr, len int32) {
        // Read from WASM memory and log
    })
    linker.FuncWrap("env", "epbf_emit_counter", func(...) {
        // Emit Prometheus metric
    })
}
```

---

## 💼 Как использовать в резюме

### Для позиции Go Developer Intern
> **eBPF Monitoring Platform** — разработал платформу мониторинга на Go с использованием eBPF, WASM, Prometheus. Реализовал динамическую регистрацию метрик, plugin lifecycle management, Docker-based сборку. 4500+ строк production кода.

### Для позиции Backend Developer Intern
> **Plugin-based Monitoring System** — спроектировал и реализовал backend систему с REST API, PostgreSQL, S3. Интегрировал Prometheus для метрик, настроил graceful shutdown, error handling, structured logging.

### Для позиции DevOps Intern
> **Cloud-Native Monitoring Platform** — развернул платформу в Docker с PostgreSQL, S3. Настроил multi-stage builds, CGO compilation, ulimits, capabilities. Реализовал health checks, graceful shutdown.

---

## 🔗 Ссылки

- **GitHub:** [github.com/your-username/epbf-monitoring](https://github.com/your-username/epbf-monitoring)
- **Документация:** docs/README.md, docs/TESTING_GUIDE.md
- **Примеры плагинов:** plugins/example-container-monitor/

---

**Проект демонстрирует готовность работать с production кодом, решать сложные технические проблемы и создавать законченные продукты.** ✅
