# Container Monitor Plugin

eBPF плагин для мониторинга событий контейнеров Docker.

## Возможности

- 🚀 Отслеживание запуска контейнеров (execve)
- 🛑 Отслеживание остановки контейнеров (kill signal)
- 🌐 Мониторинг сетевых подключений
- 📊 Метрики в реальном времени
- 🔔 События через ring buffer

## Метрики

| Метрика | Тип | Описание | Labels |
|---------|-----|----------|--------|
| `container_starts_total` | Counter | Всего запусков контейнеров | container_id, image, command |
| `container_stops_total` | Counter | Всего остановок контейнеров | container_id, signal |
| `network_connections_total` | Counter | Всего сетевых подключений | container_id, dest_ip, dest_port |
| `active_containers` | Gauge | Активные контейнеры | - |

## Фильтры

```promql
# Запуски контейнеров в минуту
per_minute(container_starts_total)

# Сетевые подключения в секунду
rate(network_connections_total[1m])

# Среднее число активных контейнеров
avg(active_containers)
```

## Сборка

### Требования

- Clang 14+ с поддержкой BPF
- Clang с поддержкой WASM32
- Linux headers

### Команды

```bash
# Локальная сборка
./build.sh

# Или через make
make build
```

### Артефакты

После сборки в директории `build/`:
- `program.o` - eBPF объект
- `plugin.wasm` - WASM модуль

## Установка

### Вариант 1: Через API

```bash
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d '{
    "git_url": "https://github.com/your-org/container-monitor-plugin.git"
  }'
```

### Вариант 2: Локально

```bash
# Скопируйте артефакты в директорию плагинов
cp build/program.o /path/to/epbf-monitoring/plugins/
cp build/plugin.wasm /path/to/epbf-monitoring/plugins/

# Или используйте file:// URL
curl -X POST http://localhost:8080/api/v1/plugins \
  -H "Content-Type: application/json" \
  -d "{\"git_url\": \"file://$(pwd)\"}"
```

## Структура

```
container-monitor/
├── manifest.yml          # Метаданные плагина
├── ebpf/
│   └── main.c           # eBPF программа
├── wasm/
│   └── main.c           # WASM обработчик
├── build/               # Артефакты сборки
├── build.sh            # Скрипт сборки
└── README.md           # Этот файл
```

## eBPF программы

### trace_container_start
- **Hook:** `tracepoint/syscalls/sys_enter_execve`
- **Описание:** Перехватывает запуск новых процессов (контейнеров)
- **Данные:** PID, PPID, comm, filename

### trace_container_stop
- **Hook:** `tracepoint/syscalls/sys_enter_kill`
- **Описание:** Перехватывает сигналы остановки
- **Данные:** PID, signal number

### trace_network_connect
- **Hook:** `tracepoint/syscalls/sys_enter_connect`
- **Описание:** Перехватывает сетевые подключения
- **Данные:** PID, comm

## Отладка

### Просмотр событий eBPF

```bash
# Через bpftool
bpftool prog dump log <program_id>

# Через trace_pipe
cat /sys/kernel/debug/tracing/trace_pipe
```

### Логи WASM

WASM плагин логирует события через `epbf_log()`. Логи доступны в stdout epbf-monitor.

## Лицензия

MIT
