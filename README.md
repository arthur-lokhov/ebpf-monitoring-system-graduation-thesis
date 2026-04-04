# epbf-monitoring

**eBPF Monitoring Platform** — модульная платформа мониторинга на основе eBPF с поддержкой пользовательских плагинов, распространяемых через Git-репозитории.

## 🚀 Возможности

- **eBPF мониторинг** — сборка и загрузка eBPF-программ на уровне ядра Linux
- **Docker-сборка** — плагины компилируются в изолированных контейнерах
- **Git-репозитории** — плагины распространяются как Git-репозитории
- **Prometheus совместимость** — экспорт метрик в формате Prometheus
- **React UI** — веб-интерфейс с дашбордами, фильтрами и графиками в реальном времени
- **Rootless режим** — работа без root-прав благодаря проверке eBPF verifier'ом
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
│  │  Manager    │  │   Browser   │  │    (Recharts)       │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────┬───────────────────────────────────────┘
                      │ REST API + WebSocket
┌─────────────────────▼───────────────────────────────────────┐
│                  API Server (Go + chi)                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Plugin    │  │   Filter    │  │     Metrics         │  │
│  │   Service   │  │   Engine    │  │     Service         │  │
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘  │
└─────────┼────────────────┼────────────────────┼─────────────┘
          │                │                    │
   ┌──────▼──────┐  ┌─────▼──────┐  ┌──────────▼──────────┐
   │   Plugin    │  │   eBPF     │  │   Prometheus        │
   │   Builder   │  │   Runtime  │  │   Scraper           │
   │  (Docker)   │  │  (native)  │  │   (:9090)           │
   └──────┬──────┘  └─────┬──────┘  └──────────┬──────────┘
          │                │                    │
          └────────────────┼────────────────────┘
                           │
              ┌────────────▼────────────┐
              │   PostgreSQL + S3       │
              │   (:5432)    (:3900)    │
              │   (metadata + blobs)    │
              └─────────────────────────┘
```

## 🚀 Быстрый старт

### Требования

- Linux 5.8+ (для eBPF)
- Go 1.21+
- Docker и Docker Compose
- Clang 14+ (для сборки плагинов в Docker-контейнере)

### Запуск

1. **Клонируйте репозиторий**

```bash
git clone https://github.com/epbf-monitoring/epbf-monitoring.git
cd epbf-monitoring
```

2. **Запустите зависимости (PostgreSQL + Garage S3 + Prometheus)**

```bash
make docker-up
```

3. **Инициализируйте базу данных**

```bash
make migrate-run
```

4. **Настройте Garage S3 — создайте бакет и ключ**

```bash
# Создайте бакет
docker exec epbf-garage /garage bucket create epbf-plugins

# Создайте ключ доступа с правами администратора
docker exec epbf-garage /garage key create --admin epbf-admin
```

**Запишите Key ID и Secret key — они понадобятся для настройки!**

5. **Выдайте ключу права на бакет**

```bash
docker exec epbf-garage /garage bucket allow \
  --read --write --owner \
  epbf-plugins \
  --key GKxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

6. **Обновите переменные окружения**

Скопируйте `deployments/.env.example` в `deployments/.env` и укажите ваши S3 credentials:

```
S3_ACCESS_KEY=GKxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
S3_SECRET_KEY=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

7. **Запустите сервер**

```bash
make run-dev
```

Приложение автоматически загружает переменные окружения из `deployments/.env`.

8. **Проверьте работу**

```bash
curl http://localhost:8080/health
# {"status":"ok"}

curl http://localhost:8080/metrics
# epbf_info{version="0.1.0"} 1
```

### Добавление первого плагина

```bash
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{"git_url": "https://github.com/epbf-monitoring/plugin-network.git"}'
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
├── filters.yml          # Предустановленные фильтры (опционально)
└── dashboard.json       # Конфиг дашборда (опционально)
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

events:
  - type: 0
    name: tcp_connection
    metric: tcp_connections_total

metrics:
  - name: tcp_connections_total
    type: counter
    help: Total TCP connections
    labels: [dest_ip, dest_port]
```

### eBPF программа (C)

```c
#include <vmlinux.h>
#include <bpf/bpf_helpers.h>

// Ring buffer для отправки событий
struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} events SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_connect")
int tcp_connect(void *ctx) {
    // Ваша логика отслеживания соединений
    return 0;
}

char __license[] SEC("license") = "GPL";
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
| `POST` | `/api/v1/plugins/{id}/enable` | Включить плагин |
| `POST` | `/api/v1/plugins/{id}/disable` | Отключить плагин |
| `POST` | `/api/v1/plugins/{id}/rebuild` | Пересобрать плагин |

### Метрики

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `GET` | `/api/v1/metrics` | Список метрик |
| `GET` | `/api/v1/metrics/{name}` | Детали метрики |
| `POST` | `/api/v1/metrics/query` | Выполнить PromQL-запрос |
| `GET` | `/api/v1/metrics/names` | Список имён метрик |

### Фильтры

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `GET` | `/api/v1/filters` | Список фильтров |
| `POST` | `/api/v1/filters` | Создать фильтр |
| `DELETE` | `/api/v1/filters/{id}` | Удалить фильтр |
| `POST` | `/api/v1/filters/execute` | Выполнить фильтр |

### Дашборды

| Метод | Endpoint | Описание |
|-------|----------|----------|
| `GET` | `/api/v1/dashboard` | Получить дашборд |
| `POST` | `/api/v1/dashboard` | Создать дашборд |
| `PUT` | `/api/v1/dashboard` | Обновить дашборд |
| `DELETE` | `/api/v1/dashboard/{id}` | Удалить дашборд |

### WebSocket

```
ws://localhost:8080/ws
```

Подписка на обновления метрик в реальном времени.

### Prometheus

```bash
# Экспорт метрик
curl http://localhost:8080/metrics

# Интеграция с Prometheus (уже настроена)
# В prometheus.yml:
scrape_configs:
  - job_name: 'epbf-monitor'
    static_configs:
      - targets: ['172.17.0.1:8080']
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
# Запуск всех сервисов (DB, S3, Prometheus, UI)
make docker-up

# Остановка
make docker-down

# Логи
make docker-logs
```

### UI разработка

```bash
make ui-dev
```

Откройте http://localhost:3000

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
| `S3_REGION` | S3 region | `garage` |
| `ENABLE_DOCKER` | Включить Docker-сборку | `true` |
| `BUILD_DIR` | Директория для сборки | `/tmp/epbf-builds` |
| `LOG_LEVEL` | Уровень логирования | `info` |

## 📚 Документация

- [docs/tech-app.md](docs/tech-app.md) — техническая документация (отчёт по практике)
- [docs/plan.md](docs/plan.md) — архитектурный план
- [docs/roadmap.md](docs/roadmap.md) — план реализации по фазам
- [deployments/RUNNING.md](deployments/RUNNING.md) — руководство по запуску

## 🛡 Безопасность

- **eBPF verifier** — все eBPF программы проходят проверку ядром Linux
- **Docker-изоляция** — плагины собираются в контейнерах с ограничением ресурсов (512MB RAM, 1 CPU)
- **Rootless режим** — работа без root-прав благодаря проверке eBPF verifier'ом
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
- [Cilium eBPF](https://github.com/cilium/ebpf)
- [Prometheus](https://prometheus.io/docs/)
- [Grafana](https://grafana.com/docs/grafana/latest/)
- [Garage S3](https://garagehq.deuxfleurs.fr/)
