# epbf-monitoring

**eBPF Monitoring Platform** — модульная платформа мониторинга на основе eBPF с поддержкой пользовательских плагинов, распространяемых через Git-репозитории.

## 🚀 Возможности

- **eBPF на основе** — мониторинг на уровне ядра Linux без влияния на производительность
- **WASM плагины** — безопасное выполнение пользовательского кода в песочнице
- **Git-репозитории** — плагины распространяются как Git-репозитории
- **Prometheus совместимость** — экспорт метрик в формате Prometheus
- **Grafana Scenes** — встроенная визуализация через Grafana Scenes в React UI
- **Rootless режим** — работа без root-прав благодаря WASM изоляции
- **PromQL фильтрация** — встроенный DSL для фильтрации метрик (rate, sum, per_second, etc.)

## 📋 Содержание

- [Архитектура](#архитектура)
- [Быстрый старт](#быстрый-старт)
- [Создание плагинов](#создание-плагинов)
- [API](#api)
- [Разработка](#разработка)
- [Документация](#документация)

## 🏗 Архитектура

```
┌─────────────────────────────────────────────────────────────┐
│                     React UI (shadcn/ui)                     │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │  Plugins    │  │   Metrics   │  │    Dashboard        │  │
│  │  Manager    │  │   Browser   │  │    (Grafana Scenes) │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────┬───────────────────────────────────────┘
                      │ REST API + WebSocket
┌─────────────────────▼───────────────────────────────────────┐
│                  API Server (Go + chi)                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Plugin    │  │   Filter    │  │     Metrics         │  │
│  │   Service   │  │   Engine    │  │     Service         │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────┬───────────────────┬───────────────────┬───────────┘
          │                   │                   │
┌─────────▼────────┐ ┌────────▼───────┐ ┌────────▼───────────┐
│  Plugin Builder  │ │  WASM Runtime  │ │   eBPF Loader      │
│  (clang + LLVM)  │ │  (wasmtime)    │ │   (libbpf/cilium)  │
└──────────────────┘ └────────────────┘ └────────────────────┘
          │                   │                   │
          └───────────────────┼───────────────────┘
                              │
                  ┌───────────▼───────────┐
                  │   PostgreSQL + S3     │
                  │   (metadata + blobs)  │
                  └───────────────────────┘
```

## 🚀 Быстрый старт

### Требования

- Linux 5.8+ (для eBPF)
- Go 1.21+
- Docker и Docker Compose
- Clang 14+ (для сборки плагинов)

### Запуск

1. **Клонируйте репозиторий**

```bash
git clone https://github.com/epbf-monitoring/epbf-monitoring.git
cd epbf-monitoring
```

2. **Запустите зависимости (PostgreSQL + Garage S3)**

```bash
make docker-up
```

3. **Запустите сервер**

```bash
make run
```

4. **Проверьте работу**

```bash
curl http://localhost:8080/health
# {"status":"ok","timestamp":"..."}

curl http://localhost:8080/metrics
# epbf_info{version="0.1.0"} 1
```

### Добавление первого плагина

```bash
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{
    "git_url": "https://github.com/epbf-monitoring/plugin-network.git"
  }'
```

## 📦 Создание плагинов

### Структура плагина

```
plugin-example/
├── manifest.yml          # Метаданные плагина
├── ebpf/
│   ├── main.c           # eBPF программа
│   └── Makefile
├── wasm/
│   ├── main.c           # WASM логика
│   └── Makefile
├── filters.yml          # Предустановленные фильтры
└── dashboard.json       # Конфиг дашборда
```

### manifest.yml

```yaml
name: network-monitor
version: 1.0.0
description: Мониторинг сетевых соединений
author: Your Name

ebpf:
  entry: ebpf/main.c
  programs:
    - name: tcp_connect
      type: tracepoint
      attach: sys_enter_connect

wasm:
  entry: wasm/main.c
  sdk_version: "1.0"

metrics:
  - name: tcp_connections_total
    type: counter
    help: Total TCP connections
    labels: [dest_ip, dest_port]

filters:
  - name: connections_per_second
    expression: rate(tcp_connections_total[1m])
```

### WASM плагин (C)

```c
#include "epbf.h"

int epbf_init(void) {
    EPBF_LOG_INFO("Plugin initialized");
    return 0;
}

void process_events() {
    // Подписка на eBPF map
    int map_fd = epbf_subscribe_map("tcp_events");
    
    // Обработка событий
    while (1) {
        struct event e;
        if (read_event(map_fd, &e) > 0) {
            // Эмит метрики
            epbf_label_t labels[] = {
                {"dest_ip", e.dest_ip},
                {"dest_port", e.dest_port}
            };
            epbf_emit_counter("tcp_connections_total", 1, labels, 2);
        }
        epbf_sleep(100);
    }
}
```

### Публикация плагина

1. Создайте Git-репозиторий с плагином
2. Добавьте теги версий (`git tag v1.0.0`)
3. Предоставьте доступ к репозиторию (или сделайте public)
4. Добавьте плагин через UI или API

## 🔌 API

### Плагины

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `GET` | `/api/v1/plugins` | Список плагинов |
| `POST` | `/api/v1/plugins` | Добавить плагин |
| `GET` | `/api/v1/plugins/{id}` | Информация о плагине |
| `DELETE` | `/api/v1/plugins/{id}` | Удалить плагин |
| `POST` | `/api/v1/plugins/{id}/rebuild` | Пересобрать плагин |

### Метрики

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `GET` | `/api/v1/metrics` | Список метрик |
| `GET` | `/api/v1/metrics/{name}` | Детали метрики |

### Фильтры

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `GET` | `/api/v1/filters` | Список фильтров |
| `POST` | `/api/v1/filters` | Создать фильтр |
| `DELETE` | `/api/v1/filters/{id}` | Удалить фильтр |

### Prometheus

```bash
# Экспорт метрик
curl http://localhost:8080/metrics

# Интеграция с Prometheus
# В prometheus.yml:
scrape_configs:
  - job_name: 'epbf-monitoring'
    static_configs:
      - targets: ['localhost:8080']
```

## 🛠 Разработка

### Сборка

```bash
make build
```

### Тесты

```bash
make test
```

### Запуск с Docker

```bash
# Сборка образов
make docker-build-builder
make docker-build-runtime

# Запуск
docker-compose -f deployments/docker-compose.yml up
```

### Переменные окружения

| Переменная | Описание | По умолчанию |
|------------|----------|--------------|
| `PORT` | Порт HTTP сервера | `8080` |
| `DB_HOST` | Хост PostgreSQL | `localhost` |
| `DB_PORT` | Порт PostgreSQL | `5432` |
| `DB_USER` | Пользователь PostgreSQL | `epbf` |
| `DB_PASSWORD` | Пароль PostgreSQL | `epbf_password` |
| `DB_NAME` | Имя базы данных | `epbf` |
| `S3_ENDPOINT` | S3 endpoint | `http://localhost:3900` |
| `S3_ACCESS_KEY` | S3 access key | - |
| `S3_SECRET_KEY` | S3 secret key | - |
| `S3_BUCKET` | S3 bucket name | `epbf-plugins` |

## 📚 Документация

- [Архитектурный план](docs/plan.md) — детальное описание архитектуры
- [Roadmap](docs/roadmap.md) — план реализации по фазам
- [Введение](docs/introduction.md) — обоснование проекта

## 🛡 Безопасность

- **WASM песочница** — плагины выполняются в изолированной среде
- **eBPF верификатор** — все eBPF программы проходят проверку ядром
- **Rootless режим** — работа без root-прав
- **Ограниченные capabilities** — только `CAP_BPF` и `CAP_PERFMON`

## 📊 Фильтрация метрик

Поддерживаемые функции DSL:

- `rate(metric[duration])` — скорость изменения
- `sum(expr)` — суммирование
- `avg(expr)` — усреднение
- `per_second(metric)` — нормализация к секунде
- `per_minute(metric)` — нормализация к минуте
- `histogram_quantile(p, h)` — перцентили
- `by(labels...)` — группировка

Пример:
```
rate(tcp_connections_total[1m])
sum by (interface) (bytes_sent)
histogram_quantile(0.99, request_duration_bucket)
```

## 🤝 Вклад

1. Fork репозиторий
2. Создайте feature branch (`git checkout -b feature/amazing-feature`)
3. Закоммитьте изменения (`git commit -m 'Add amazing feature'`)
4. Запушьте (`git push origin feature/amazing-feature`)
5. Откройте Pull Request

## 📝 License

MIT License — см. [LICENSE](LICENSE) файл.

## 🔗 Ссылки

- [eBPF Documentation](https://ebpf.io/)
- [Cilium libbpf](https://github.com/cilium/libbpf)
- [Wasmtime](https://github.com/bytecodealliance/wasmtime)
- [Grafana Scenes](https://grafana.com/docs/grafana/latest/developers/scenes/)
- [Garage S3](https://garagehq.deuxfleurs.fr/)
