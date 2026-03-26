# epbf-monitoring: Архитектурный план

## 📋 Обзор проекта

**epbf-monitoring** — платформа мониторинга на основе eBPF с модульной архитектурой, поддерживающая загрузку пользовательских плагинов через Git-репозитории, экспорт метрик в Prometheus и встроенную визуализацию через Grafana Scenes.

---

## 🎯 Ключевые решения

| Компонент | Решение | Обоснование |
|-----------|---------|-------------|
| **UI** | React + shadcn/ui | Быстро, красиво, не изобретать велосипед |
| **Графики** | Grafana Scenes | Готовые визуализации, фильтрация, алерты |
| **Хранилище** | PostgreSQL + Garage S3 | PG для метаданных, S3 для WASM-бинарников |
| **Плагины** | WebAssembly | Безопасность, rootless исполнение, песочница |
| **Реестр** | Git-репозиторий | Децентрализованно, версионирование, PR/MR |
| **Фильтрация** | PromQL-подобный DSL | rate(), sum(), per_second() на лету |

---

## 🏗 Архитектура системы

```
┌──────────────────────────────────────────────────────────────────┐
│                     React UI (shadcn/ui)                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐   │
│  │  Plugins    │  │   Metrics   │  │      Dashboard          │   │
│  │  Manager    │  │   Browser   │  │      (Grafana Scenes)   │   │
│  │  + Add URL  │  │  + Filters  │  │      + PromQL           │   │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘   │
└─────────────────────┬────────────────────────────────────────────┘
                      │ REST API + WebSocket (real-time)
┌─────────────────────▼────────────────────────────────────────────┐
│                    API Server (Go)                                │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐   │
│  │   Plugin    │  │   Filter    │  │      Metrics            │   │
│  │   Controller│  │   Engine    │  │      Controller         │   │
│  └─────────────┘  └─────────────┘  └─────────────────────────┘   │
└─────────┬──────────────────┬──────────────────┬─────────────────┘
          │                  │                  │
          ▼                  ▼                  ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────────────┐
│  Plugin Builder │ │  WASM Runtime   │ │  eBPF Loader            │
│  (CI in a box)  │ │  (wasmtime)     │ │  (libbpf)               │
│  • clang (eBPF) │ │  • Sandbox      │ │  • Load .o              │
│  • clang (WASM) │ │  • Host funcs   │ │  • Attach hooks         │
│  • verifier     │ │  • Event loop   │ │  • Maps                 │
└────────┬────────┘ └────────┬────────┘ └───────────┬─────────────┘
         │                   │                       │
         ▼                   ▼                       ▼
┌──────────────────────────────────────────────────────────────────┐
│                         Storage Layer                             │
│  ┌─────────────────────┐         ┌─────────────────────────────┐ │
│  │   PostgreSQL        │         │   Garage S3                 │ │
│  │   • plugins         │         │   • plugins/<id>/ebpf.o     │ │
│  │   • metrics         │         │   • plugins/<id>/wasm.wasm  │ │
│  │   • filters         │         │   • plugin-cache/           │ │
│  │   • dashboards      │         │                             │ │
│  └─────────────────────┘         └─────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────┐
│                      /metrics endpoint                            │
│  Prometheus format с поддержкой фильтров                          │
└──────────────────────────────────────────────────────────────────┘
```

---

## 📦 Структура плагина

Плагин распространяется как Git-репозиторий со следующей структурой:

```
plugin-network-monitor/
├── manifest.yml                 # Обязательный файл
├── ebpf/
│   ├── network.c               # eBPF программа (C)
│   ├── network.h               # Заголовки
│   └── Makefile                # Сборка eBPF (наш шаблон)
├── wasm/
│   ├── main.c                  # WASM логика (C)
│   └── Makefile                # Сборка WASM (наш шаблон)
├── filters.yml                 # Предустановленные фильтры (опционально)
└── dashboard.json              # Grafana Scenes конфиг (опционально)
```

### manifest.yml

```yaml
name: network-monitor
version: 1.0.0
description: Мониторинг сетевых соединений
author: John Doe

# Точки входа
ebpf:
  entry: network.c
  programs:
    - name: tcp_connect
      type: tracepoint
      attach: sys_enter_connect
    - name: tcp_send
      type: kprobe
      attach: tcp_sendmsg

wasm:
  entry: wasm/main.c
  sdk_version: 1.0

# Метрики
metrics:
  - name: tcp_connections_total
    type: counter
    help: Total TCP connections
    labels: [dest_ip, dest_port, interface]
    
  - name: bytes_sent
    type: counter
    help: Bytes sent
    labels: [dest_ip, interface]

# Фильтры (опционально)
filters:
  - name: connections_per_second
    expression: rate(tcp_connections_total[1m])
    
  - name: bytes_per_minute
    expression: per_minute(bytes_sent)
```

---

## 🔄 Поток загрузки плагина

```
1. Пользователь в UI
   └─> Вводит URL: https://github.com/user/plugin-network
   └─> Нажимает "Add Plugin"

2. Plugin Loader
   └─> git clone <url> → /tmp/plugins/<name>
   └─> Читает manifest.yml
   └─> Валидация структуры

3. Сборка (в изолированном контейнере)
   ┌───────────────────────────────────────────────────────┐
   │  make ebpf                                            │
   │    └─> clang -target bpf network.c → network.o        │
   │    └─> Проверка верификатором eBPF                    │
   │                                                       │
   │  make wasm                                            │
   │    └─> clang --target=wasm32 main.c → main.wasm       │
   │    └─> Линковка с WASM SDK                            │
   └───────────────────────────────────────────────────────┘

4. Сохранение
   └─> network.o → Garage S3
   └─> main.wasm → Garage S3
   └─> manifest.yml → PostgreSQL
   └─> filters.yml → PostgreSQL

5. Активация
   └─> Загрузка eBPF программы в ядро
   └─> Запуск WASM в песочнице
   └─> Подключение к /metrics endpoint
```

---

## 🎯 DSL для фильтрации

### Поддерживаемые функции

| Функция | Описание | Пример |
|---------|----------|--------|
| `rate(metric[duration])` | Скорость изменения за интервал | `rate(tcp_connections_total[1m])` |
| `sum(expr)` | Суммирование | `sum(rate(bytes_total[30s]))` |
| `avg(expr)` | Усреднение | `avg(cpu_usage)` |
| `per_second(metric)` | Нормализация к секунде | `per_second(bytes_sent)` |
| `per_minute(metric)` | Нормализация к минуте | `per_minute(requests_total)` |
| `histogram_quantile(p, h)` | Перцентили | `histogram_quantile(0.99, latency_bucket)` |
| `by(labels...)` | Группировка | `sum by (interface) (bytes_total)` |

### Пример конфигурации фильтров

```yaml
filters:
  - name: packets_per_second
    expression: rate(packets_received[1m])
    
  - name: bytes_per_minute
    expression: per_minute(bytes_sent)
    
  - name: total_network
    expression: sum(rate(bytes_total[30s])) by (interface)
    
  - name: error_rate
    expression: rate(errors_total[5m]) / rate(packets_total[5m])
```

---

## 🛡 Модель безопасности

### Уровни изоляции

```
┌─────────────────────────────────────────────────────────────┐
│                   Security Boundary                          │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              User Space (непривилегированный)         │   │
│  │  ┌────────────────┐  ┌────────────────────────────┐  │   │
│  │  │   WASM Plugin  │  │      Go Runtime            │  │   │
│  │  │   (sandboxed)  │  │   (epbf-monitor daemon)    │  │   │
│  │  │                │  │                            │  │   │
│  │  │  ❌ Нет syscall│  │  ✅ Имеет CAP_BPF          │  │   │
│  │  │  ❌ Нет FS     │  │  ✅ Имеет CAP_PERFMON      │  │   │
│  │  │  ❌ Нет net    │  │  ✅ Загружает eBPF         │  │   │
│  │  └────────────────┘  └────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐   │
│  │              Kernel Space (привилегированный)         │   │
│  │  ┌────────────────────────────────────────────────┐  │   │
│  │  │              eBPF Programs                      │  │   │
│  │  │  • Проходят верификацию ядра                   │  │   │
│  │  │  • Ограниченное число инструкций               │  │   │
│  │  │  • Нет циклов (или ограниченные)               │  │   │
│  │  │  • Только whitelist хуков                      │  │   │
│  │  └────────────────────────────────────────────────┘  │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Контейнер запускается без root

```yaml
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  capabilities:
    add:
      - BPF        # Для загрузки eBPF программ
      - PERFMON    # Для eBPF perf events
      - SYS_RESOURCE # Для rlimit
    drop:
      - ALL        # Всё остальное отключено
  
  # Дополнительно
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  seccompProfile:
    type: RuntimeDefault
```

---

## 🗄 Схема базы данных (PostgreSQL)

```sql
-- Плагины
CREATE TABLE plugins (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    version VARCHAR(50) NOT NULL,
    description TEXT,
    author VARCHAR(255),
    git_url TEXT NOT NULL,
    git_commit VARCHAR(40),
    ebpf_s3_key TEXT,
    wasm_s3_key TEXT,
    manifest JSONB NOT NULL,
    status VARCHAR(50) DEFAULT 'pending', -- pending, building, ready, error
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Метрики
CREATE TABLE metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id UUID REFERENCES plugins(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- counter, gauge, histogram
    help TEXT,
    labels JSONB DEFAULT '[]',
    UNIQUE(plugin_id, name)
);

-- Фильтры
CREATE TABLE filters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id UUID REFERENCES plugins(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    expression TEXT NOT NULL,
    description TEXT,
    is_default BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(plugin_id, name)
);

-- Дашборды
CREATE TABLE dashboards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    config JSONB NOT NULL, -- Grafana Scenes config
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

---

## 📁 Структура проекта

```
epbf-monitoring/
├── docs/
│   ├── introduction.md
│   ├── plan.md              # Этот файл
│   └── roadmap.md           # План реализации
│
├── cmd/
│   └── epbf-monitor/
│       └── main.go          # Точка входа
│
├── internal/
│   ├── api/
│   │   ├── handlers/
│   │   │   ├── plugins.go
│   │   │   ├── metrics.go
│   │   │   ├── filters.go
│   │   │   └── dashboard.go
│   │   ├── router.go
│   │   └── websocket.go
│   │
│   ├── plugin/
│   │   ├── loader.go
│   │   ├── manifest.go
│   │   ├── validator.go
│   │   └── builder/
│   │       ├── builder.go
│   │       ├── ebpf.go
│   │       ├── wasm.go
│   │       └── verifier.go
│   │
│   ├── runtime/
│   │   ├── ebpf/
│   │   │   ├── loader.go
│   │   │   ├── maps.go
│   │   │   └── programs.go
│   │   ├── wasm/
│   │   │   ├── runtime.go
│   │   │   ├── sandbox.go
│   │   │   └── host_funcs.go
│   │   └── metrics/
│   │       ├── collector.go
│   │       ├── registry.go
│   │       └── exporter.go
│   │
│   ├── filter/
│   │   ├── engine.go
│   │   ├── functions.go
│   │   ├── parser.go
│   │   └── executor.go
│   │
│   ├── storage/
│   │   ├── postgres/
│   │   │   ├── plugin_repo.go
│   │   │   ├── metric_repo.go
│   │   │   ├── filter_repo.go
│   │   │   └── migrations/
│   │   └── s3/
│   │       ├── client.go
│   │       └── storage.go
│   │
│   └── dashboard/
│       ├── scenes.go
│       ├── builder.go
│       └── templates.go
│
├── pkg/
│   └── wasmsdk/
│       ├── include/
│       │   └── epbf.h
│       └── src/
│           └── epbf.c
│
├── build/
│   ├── docker/
│   │   ├── builder.Dockerfile
│   │   └── runtime.Dockerfile
│   └── templates/
│       ├── ebpf-Makefile
│       └── wasm-Makefile
│
├── ui/
│   ├── src/
│   │   ├── components/
│   │   │   ├── ui/
│   │   │   ├── plugins/
│   │   │   ├── metrics/
│   │   │   └── dashboard/
│   │   ├── lib/
│   │   │   ├── api.ts
│   │   │   ├── websocket.ts
│   │   │   └── scenes.ts
│   │   ├── hooks/
│   │   └── App.tsx
│   ├── components.json
│   └── package.json
│
├── deployments/
│   ├── docker-compose.yml
│   └── kubernetes/
│
├── plugins/
│   ├── network/
│   └── disk/
│
├── go.mod
├── Makefile
└── README.md
```

---

## 🛠 Технологический стек

| Слой | Технология |
|------|------------|
| **eBPF** | C + libbpf + Clang 14+ |
| **WASM Runtime** | Go + wasmtime-go |
| **Core** | Go 1.21+ |
| **DB** | PostgreSQL 15+ |
| **S3** | Garage (S3-compatible) |
| **UI** | React 18 + TypeScript + shadcn/ui |
| **Визуализация** | Grafana Scenes (встроенная) |
| **Контейнеры** | Docker + rootless mode |

---

## 📊 Поток данных

```
1. Сбор данных
   eBPF (ядро) → eBPF Maps → WASM Runtime → Metrics Registry

2. Применение фильтров
   Raw Metrics → Filter Engine (rate, sum, etc.) → Transformed Metrics

3. Визуализация
   Transformed Metrics → Grafana Scenes (React) → Charts

4. Экспорт
   Transformed Metrics → /metrics → Prometheus / External Grafana
```

---

## 🔧 WASM SDK API

```c
// epbf.h - SDK для написания WASM части плагина

#ifndef EPBF_WASM_SDK_H
#define EPBF_WASM_SDK_H

#include <stdint.h>
#include <stddef.h>

// Типы метрик
typedef enum {
    METRIC_COUNTER,
    METRIC_GAUGE,
    METRIC_HISTOGRAM
} metric_type_t;

// Структура метки
typedef struct {
    const char* key;
    const char* value;
} label_t;

// Инициализация плагина
int epbf_init(void);

// Подписка на eBPF map
int epbf_subscribe_map(const char* map_name);

// Чтение из map
int epbf_read_map(const char* map_name, const void* key, size_t key_size, 
                  void* value, size_t value_size);

// Эмит метрики
void epbf_emit_counter(const char* name, uint64_t value, 
                       label_t* labels, size_t label_count);

void epbf_emit_gauge(const char* name, double value,
                     label_t* labels, size_t label_count);

void epbf_emit_histogram(const char* name, double* buckets, uint64_t* counts,
                         size_t bucket_count, label_t* labels, size_t label_count);

// Логирование
void epbf_log(const char* level, const char* message);

// Таймеры
uint64_t epbf_now_ns(void);  // Текущее время в наносекундах

#endif
```

---

## 🌐 API Endpoints

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `GET` | `/api/v1/plugins` | Список плагинов |
| `POST` | `/api/v1/plugins` | Добавить плагин (URL репозитория) |
| `GET` | `/api/v1/plugins/:id` | Информация о плагине |
| `DELETE` | `/api/v1/plugins/:id` | Удалить плагин |
| `POST` | `/api/v1/plugins/:id/rebuild` | Пересобрать плагин |
| `GET` | `/api/v1/metrics` | Браузер метрик |
| `GET` | `/api/v1/metrics/:name` | Детали метрики |
| `GET` | `/api/v1/filters` | Список фильтров |
| `POST` | `/api/v1/filters` | Создать фильтр |
| `DELETE` | `/api/v1/filters/:id` | Удалить фильтр |
| `GET` | `/api/v1/dashboard` | Конфиг дашборда |
| `PUT` | `/api/v1/dashboard` | Обновить дашборд |
| `GET` | `/metrics` | Prometheus формат (внешний) |
| `WS` | `/ws` | WebSocket для real-time обновлений |

---

## 📝 Примечания

### Почему WASM без доступа к сети/ФС?

WASM-плагин **не имеет прямого доступа** к сети/ФС хоста, но получает данные о них через eBPF, который работает в ядре и отслеживает все системные вызовы. Это обеспечивает безопасность: даже скомпрометированный плагин не может навредить системе.

### Почему Git-репозиторий как реестр?

- Децентрализация — нет единой точки отказа
- Версионирование — теги и коммиты
- Code review — PR/MR для проверки кода
- CI/CD — автоматическая сборка и тестирование

### Почему Grafana Scenes вместо полной Grafana?

- Легковесность — не нужно разворачивать отдельную Grafana
- Интеграция — графики прямо в UI системы
- Гибкость — экспорт в внешнюю Grafana через `/metrics`

---

## 📚 Ссылки

- [eBPF Documentation](https://ebpf.io/)
- [libbpf](https://github.com/libbpf/libbpf)
- [wasmtime-go](https://github.com/bytecodealliance/wasmtime-go)
- [shadcn/ui](https://ui.shadcn.com/)
- [Grafana Scenes](https://grafana.com/docs/grafana/latest/developers/scenes/)
- [Garage S3](https://garagehq.deuxfleurs.fr/)
