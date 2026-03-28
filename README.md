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
cd deployments/
export DOCKER_HOST="unix:///Users/asa/.colima/default/docker.sock"  # Для macOS/Colima
docker-compose up -d
```

3. **Инициализируйте базу данных**

```bash
docker exec -i epbf-postgres psql -U epbf -d epbf < ../internal/storage/postgres/migrations/001_init_schema.up.sql
```

Ожидаемый вывод:
```
CREATE EXTENSION
CREATE TABLE
CREATE INDEX
...
INSERT 0 1
```

4. **Настройте Garage S3 - создайте бакет и ключ**

```bash
# Создайте бакет
docker exec epbf-garage /garage bucket create epbf-plugins

# Создайте ключ доступа с правами администратора
docker exec epbf-garage /garage key create --admin epbf-admin
```

Ожидаемый вывод:
```
==== ACCESS KEY INFORMATION ====
Key ID:              GKxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
Key name:            epbf-admin
Secret key:          xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
Created:             ...
Validity:            valid
Can create buckets:  true
```

**Запишите Key ID и Secret key - они понадобятся для настройки!**

5. **Выдайте ключу права на бакет**

```bash
# Замените GKxxx на ваш Key ID из предыдущего шага
docker exec epbf-garage /garage bucket allow \
  --read --write --owner \
  epbf-plugins \
  --key GKxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

6. **Обновите переменные окружения в docker-compose.yml**

Откройте `deployments/docker-compose.yml` и найдите секцию `epbf-monitor`:

```yaml
epbf-monitor:
  environment:
    S3_ACCESS_KEY: GKxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  # Ваш Key ID
    S3_SECRET_KEY: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  # Ваш Secret key
```

7. **Перезапустите сервер с новыми credentials**

```bash
docker-compose restart epbf-monitor
```

8. **Запустите сервер** (если не используете docker-compose)

```bash
make run
```

9. **Проверьте работу**

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

Или используйте тестовый плагин из репозитория:

```bash
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "file:///workspace/plugins/example-container-monitor"}'
```

Проверьте статус сборки:

```bash
curl http://localhost:8080/api/v1/plugins/<plugin-id> | python3 -m json.tool
```

Ожидаемый статус после успешной сборки: `"status": "ready"`

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
docker build -f deployments/docker/builder.Dockerfile -t epbf-monitor-builder:latest .
docker build -f deployments/docker/runtime.Dockerfile -t epbf-monitor-runtime:latest .

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
