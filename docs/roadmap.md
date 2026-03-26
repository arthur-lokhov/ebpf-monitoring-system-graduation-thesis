# epbf-monitoring: Roadmap

Реализация платформы мониторинга на основе eBPF с модульной архитектурой и поддержкой плагинов.

---

## 📅 Фазы проекта

### Фаза 1: Foundation (Недели 1-2)
**Цель:** Базовая инфраструктура и каркас приложения

- [ ] **1.1** Инициализация проекта
  - [ ] Создать структуру директорий
  - [ ] Настроить `go.mod` с необходимыми зависимостями
  - [ ] Создать базовый `Makefile`
  - [ ] Настроить `.gitignore`

- [ ] **1.2** Docker окружение
  - [ ] `docker-compose.yml` для локальной разработки (PostgreSQL + Garage)
  - [ ] `build/docker/builder.Dockerfile` для сборки плагинов
  - [ ] `build/docker/runtime.Dockerfile` для рантайма

- [ ] **1.3** База данных
  - [ ] Создать SQL миграции (`internal/storage/postgres/migrations/`)
  - [ ] Настроить подключение к PostgreSQL
  - [ ] Реализовать health check БД

- [ ] **1.4** S3 хранилище
  - [ ] Настроить клиент Garage S3
  - [ ] Реализовать загрузку/скачивание файлов
  - [ ] Health check S3

- [ ] **1.5** Базовый API сервер
  - [ ] HTTP роутер (chi/gin)
  - [ ] Health endpoint (`/health`)
  - [ ] Логирование запросов

**Результат фазы:** Работающий каркас с БД и S3

---

### Фаза 2: Plugin System (Недели 3-4)
**Цель:** Загрузка и сборка плагинов из Git-репозиториев

- [ ] **2.1** Plugin Loader
  - [ ] `internal/plugin/loader.go` — загрузка из Git
  - [ ] Клонирование репозитория во временную директорию
  - [ ] Поддержка тегов и коммитов
  - [ ] Очистка кэша

- [ ] **2.2** Manifest Parser
  - [ ] `internal/plugin/manifest.go` — парсинг YAML
  - [ ] Валидация структуры manifest.yml
  - [ ] Проверка обязательных полей

- [ ] **2.3** Plugin Validator
  - [ ] `internal/plugin/validator.go` — валидация
  - [ ] Проверка наличия файлов eBPF и WASM
  - [ ] Валидация имён метрик

- [ ] **2.4** Plugin Builder — eBPF
  - [ ] `internal/plugin/builder/ebpf.go`
  - [ ] Запуск clang с `-target bpf`
  - [ ] Обработка ошибок компиляции
  - [ ] Сохранение `.o` в S3

- [ ] **2.5** Plugin Builder — WASM
  - [ ] `internal/plugin/builder/wasm.go`
  - [ ] Запуск clang с `--target=wasm32`
  - [ ] Линковка с WASM SDK
  - [ ] Сохранение `.wasm` в S3

- [ ] **2.6** Build Orchestrator
  - [ ] `internal/plugin/builder/builder.go`
  - [ ] Запуск сборки в Docker контейнере
  - [ ] Логирование процесса сборки
  - [ ] Обработка ошибок

- [ ] **2.7** eBPF Verifier
  - [ ] `internal/plugin/builder/verifier.go`
  - [ ] Проверка eBPF байт-кода
  - [ ] Ограничение на число инструкций
  - [ ] Whitelist хуков

- [ ] **2.8** Plugin Repository (PostgreSQL)
  - [ ] `internal/storage/postgres/plugin_repo.go`
  - [ ] CRUD операции для плагинов
  - [ ] Статусы: pending, building, ready, error

**Результат фазы:** Работающая загрузка и сборка плагинов

---

### Фаза 3: Runtime (Недели 5-6)
**Цель:** Запуск eBPF программ и WASM модулей

- [ ] **3.1** WASM SDK
  - [ ] `pkg/wasmsdk/include/epbf.h` — C заголовки
  - [ ] `pkg/wasmsdk/src/epbf.c` — реализация
  - [ ] Документация для разработчиков плагинов

- [ ] **3.2** WASM Runtime
  - [ ] `internal/runtime/wasm/runtime.go`
  - [ ] Интеграция wasmtime-go
  - [ ] Загрузка WASM модулей
  - [ ] Управление жизненным циклом

- [ ] **3.3** WASM Sandbox
  - [ ] `internal/runtime/wasm/sandbox.go`
  - [ ] Ограничение памяти
  - [ ] Ограничение CPU
  - [ ] Изоляция между плагинами

- [ ] **3.4** Host Functions
  - [ ] `internal/runtime/wasm/host_funcs.go`
  - [ ] `epbf_subscribe_map()` — подписка на eBPF map
  - [ ] `epbf_read_map()` — чтение из map
  - [ ] `epbf_emit_counter()` — эмит метрик
  - [ ] `epbf_emit_gauge()` — эмит метрик
  - [ ] `epbf_log()` — логирование
  - [ ] `epbf_now_ns()` — время

- [ ] **3.5** eBPF Loader
  - [ ] `internal/runtime/ebpf/loader.go`
  - [ ] Интеграция libbpf
  - [ ] Загрузка eBPF программ в ядро
  - [ ] Привязка к хукам (kprobe, tracepoint)

- [ ] **3.6** eBPF Maps
  - [ ] `internal/runtime/ebpf/maps.go`
  - [ ] Создание maps
  - [ ] Чтение/запись данных
  - [ ] Ring buffer для событий

- [ ] **3.7** eBPF Programs
  - [ ] `internal/runtime/ebpf/programs.go`
  - [ ] Управление жизненным циклом
  - [ ] Attach/detach хуков
  - [ ] Статистика выполнения

- [ ] **3.8** Metrics Collector
  - [ ] `internal/runtime/metrics/collector.go`
  - [ ] Сбор данных от WASM плагинов
  - [ ] Агрегация метрик
  - [ ] Буферизация

- [ ] **3.9** Metrics Registry
  - [ ] `internal/runtime/metrics/registry.go`
  - [ ] Реестр активных метрик
  - [ ] Связь с плагинами
  - [ ] Метрики по типам (counter, gauge, histogram)

**Результат фазы:** Работающий рантайм для eBPF + WASM

---

### Фаза 4: Filter Engine (Неделя 7)
**Цель:** PromQL-подобный DSL для фильтрации метрик

- [ ] **4.1** DSL Parser
  - [ ] `internal/filter/parser.go`
  - [ ] Лексический анализ
  - [ ] Синтаксический разбор
  - [ ] AST представление

- [ ] **4.2** Filter Functions
  - [ ] `internal/filter/functions.go`
  - [ ] `rate(metric[duration])`
  - [ ] `sum(expr)`
  - [ ] `avg(expr)`
  - [ ] `per_second(metric)`
  - [ ] `per_minute(metric)`
  - [ ] `histogram_quantile(p, h)`
  - [ ] `by(labels...)`

- [ ] **4.3** Filter Engine
  - [ ] `internal/filter/engine.go`
  - [ ] Применение фильтров к метрикам
  - [ ] Кэширование результатов
  - [ ] Окна агрегации

- [ ] **4.4** Filter Executor
  - [ ] `internal/filter/executor.go`
  - [ ] Выполнение вычислений
  - [ ] Обработка ошибок
  - [ ] Таймауты

- [ ] **4.5** Filter Repository
  - [ ] `internal/storage/postgres/filter_repo.go`
  - [ ] CRUD для фильтров
  - [ ] Предустановленные фильтры из плагинов

**Результат фазы:** Работающая система фильтрации

---

### Фаза 5: Metrics Export (Неделя 8)
**Цель:** Экспорт метрик в формате Prometheus

- [ ] **5.1** Metrics Exporter
  - [ ] `internal/runtime/metrics/exporter.go`
  - [ ] Форматирование в Prometheus text format
  - [ ] Поддержка всех типов метрик
  - [ ] HELP и TYPE директивы

- [ ] **5.2** HTTP Endpoint /metrics
  - [ ] `internal/api/handlers/metrics.go`
  - [ ] Обработчик GET /metrics
  - [ ] Применение фильтров перед экспортом
  - [ ] Content-Type: text/plain; version=0.0.4

- [ ] **5.3** Metrics Browser API
  - [ ] `GET /api/v1/metrics` — список метрик
  - [ ] `GET /api/v1/metrics/:name` — детали
  - [ ] Поиск и фильтрация
  - [ ] Пагинация

- [ ] **5.4** Real-time Updates
  - [ ] `internal/api/websocket.go`
  - [ ] WebSocket сервер
  - [ ] Подписка на обновления метрик
  - [ ] Push уведомлений клиентам

**Результат фазы:** Экспорт метрик и real-time обновления

---

### Фаза 6: UI — Plugins & Metrics (Недели 9-10)
**Цель:** Интерфейс управления плагинами и браузинга метрик

- [ ] **6.1** UI Setup
  - [ ] Инициализация React проекта
  - [ ] Настройка TypeScript
  - [ ] Установка shadcn/ui
  - [ ] Настройка Tailwind CSS
  - [ ] Базовая структура компонентов

- [ ] **6.2** API Client
  - [ ] `ui/src/lib/api.ts` — HTTP клиент
  - [ ] `ui/src/lib/websocket.ts` — WebSocket клиент
  - [ ] Типы TypeScript для API
  - [ ] Обработка ошибок

- [ ] **6.3** Plugin List
  - [ ] `ui/src/components/plugins/plugin-list.tsx`
  - [ ] Таблица плагинов
  - [ ] Статусы (pending, building, ready, error)
  - [ ] Кнопки действий

- [ ] **6.4** Add Plugin
  - [ ] `ui/src/components/plugins/plugin-add.tsx`
  - [ ] Форма с URL репозитория
  - [ ] Валидация URL
  - [ ] Прогресс сборки (WebSocket)

- [ ] **6.5** Plugin Card
  - [ ] `ui/src/components/plugins/plugin-card.tsx`
  - [ ] Детали плагина
  - [ ] Список метрик
  - [ ] Логи сборки
  - [ ] Удаление/пересборка

- [ ] **6.6** Metrics Browser
  - [ ] `ui/src/components/metrics/metrics-browser.tsx`
  - [ ] Поиск метрик
  - [ ] Фильтры по типу
  - [ ] Карточки метрик

- [ ] **6.7** Filter Editor
  - [ ] `ui/src/components/metrics/filter-editor.tsx`
  - [ ] Редактор выражений
  - [ ] Автодополнение функций
  - [ ] Предпросмотр результатов

- [ ] **6.8** Custom Hooks
  - [ ] `ui/src/hooks/usePlugins.ts`
  - [ ] `ui/src/hooks/useMetrics.ts`
  - [ ] `ui/src/hooks/useWebSocket.ts`

**Результат фазы:** Работающий UI для управления плагинами и метриками

---

### Фаза 7: UI — Dashboard (Недели 11-12)
**Цель:** Визуализация через Grafana Scenes

- [ ] **7.1** Grafana Scenes Integration
  - [ ] `ui/src/lib/scenes.ts`
  - [ ] Настройка Grafana Scenes
  - [ ] Интеграция с данными системы

- [ ] **7.2** Dashboard Component
  - [ ] `ui/src/components/dashboard/dashboard.tsx`
  - [ ] Сетка дашборда
  - [ ] Drag-and-drop виджетов
  - [ ] Сохранение конфигурации

- [ ] **7.3** Scene Grid
  - [ ] `ui/src/components/dashboard/scene-grid.tsx`
  - [ ] Адаптивная сетка
  - [ ] Ресайз виджетов

- [ ] **7.4** Scene Chart
  - [ ] `ui/src/components/dashboard/scene-chart.tsx`
  - [ ] Графики на основе Scenes
  - [ ] Настройка осей
  - [ ] Легенда

- [ ] **7.5** Scene Config
  - [ ] `ui/src/components/dashboard/scene-config.tsx`
  - [ ] Конфигурация виджета
  - [ ] Выбор метрики
  - [ ] Применение фильтров

- [ ] **7.6** Dashboard Templates
  - [ ] `internal/dashboard/templates.go`
  - [ ] Шаблоны для типовых дашбордов
  - [ ] Network, Disk, Process

- [ ] **7.7** Dashboard Builder
  - [ ] `internal/dashboard/builder.go`
  - [ ] Построение дашбордов из конфига
  - [ ] Сохранение в PostgreSQL

- [ ] **7.8** Dashboard API
  - [ ] `GET /api/v1/dashboard` — получить конфиг
  - [ ] `PUT /api/v1/dashboard` — обновить
  - [ ] `POST /api/v1/dashboard/templates` — применить шаблон

**Результат фазы:** Полноценный дашборд с графиками

---

### Фаза 8: Example Plugins (Неделя 13)
**Цель:** Примеры плагинов для тестирования

- [ ] **8.1** Network Plugin
  - [ ] `plugins/network/manifest.yml`
  - [ ] `plugins/network/ebpf/network.c` — eBPF программа
  - [ ] `plugins/network/wasm/main.c` — WASM логика
  - [ ] `plugins/network/filters.yml` — фильтры
  - [ ] Метрики: tcp_connections, bytes_sent/recv, latency

- [ ] **8.2** Disk Plugin
  - [ ] `plugins/disk/manifest.yml`
  - [ ] `plugins/disk/ebpf/disk.c`
  - [ ] `plugins/disk/wasm/main.c`
  - [ ] Метрики: read/write ops, bytes, latency

- [ ] **8.3** Process Plugin
  - [ ] `plugins/process/manifest.yml`
  - [ ] `plugins/process/ebpf/process.c`
  - [ ] `plugins/process/wasm/main.c`
  - [ ] Метрики: cpu usage, memory, exec count

**Результат фазы:** Рабочие примеры плагинов

---

### Фаза 9: Kubernetes & Deployment (Неделя 14)
**Цель:** Развёртывание в production

- [ ] **9.1** Kubernetes Manifests
  - [ ] `deployments/kubernetes/deployment.yaml`
  - [ ] `deployments/kubernetes/service.yaml`
  - [ ] `deployments/kubernetes/configmap.yaml`
  - [ ] `deployments/kubernetes/rbac.yaml` — CAP_BPF, CAP_PERFMON

- [ ] **9.2** Helm Chart (опционально)
  - [ ] `deployments/helm/epbf-monitoring/`
  - [ ] values.yaml
  - [ ] templates/

- [ ] **9.3** Production Dockerfile
  - [ ] Мультистейдж сборка
  - [ ] Минимальный образ (distroless)
  - [ ] Security hardening

- [ ] **9.4** Documentation
  - [ ] README.md с быстрым стартом
  - [ ] Документация для разработчиков плагинов
  - [ ] Deployment guide

**Результат фазы:** Готовность к production

---

### Фаза 10: Testing & Polish (Неделя 15-16)
**Цель:** Тестирование и финальная полировка

- [ ] **10.1** Unit Tests
  - [ ] Тесты для internal/*
  - [ ] Mock для eBPF и WASM
  - [ ] Покрытие > 70%

- [ ] **10.2** Integration Tests
  - [ ] Тесты с реальным eBPF
  - [ ] Тесты с реальным WASM
  - [ ] End-to-end сценарии

- [ ] **10.3** Performance Testing
  - [ ] Замер накладных расходов
  - [ ] Нагрузочное тестирование
  - [ ] Оптимизация узких мест

- [ ] **10.4** Security Audit
  - [ ] Проверка WASM песочницы
  - [ ] Аудит eBPF программ
  - [ ] Security best practices

- [ ] **10.5** UI Polish
  - [ ] Исправление багов
  - [ ] Улучшение UX
  - [ ] Accessibility

- [ ] **10.6** Documentation
  - [ ] API документация
  - [ ] User guide
  - [ ] Troubleshooting

**Результат фазы:** Готовый к релизу продукт

---

## 📊 Диаграмма Ганта

```
Фаза  │ 1  2  3  4  5  6  7  8  9  10 11 12 13 14 15 16
──────┼────────────────────────────────────────────────
F1    │ ██ ██
F2    │       ██ ██
F3    │          ██ ██
F4    │                ██
F5    │                   ██
F6    │                      ██ ██
F7    │                            ██ ██
F8    │                                  ██
F9    │                                     ██
F10   │                                        ██ ██
```

**Общая длительность:** 16 недель (~4 месяца)

---

## 🎯 MVP (Минимально жизнеспособный продукт)

Для демонстрации концепции достаточно реализовать:

- [x] Фаза 1: Foundation (полностью)
- [x] Фаза 2: Plugin System (пункты 2.1-2.3, 2.8)
- [x] Фаза 3: Runtime (пункты 3.2-3.4, 3.8-3.9)
- [x] Фаза 5: Metrics Export (пункты 5.1-5.2)
- [x] Фаза 6: UI (пункты 6.1-6.5)
- [x] Фаза 8: Network Plugin (один плагин)

**MVP длительность:** ~8 недель

---

## 📝 Заметки

### Зависимости

```bash
# Go зависимости
go get github.com/cilium/ebpf
go get github.com/bytecodealliance/wasmtime-go
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5
go get github.com/aws/aws-sdk-go-v2/service/s3
go get gopkg.in/yaml.v3

# UI зависимости
npm install @radix-ui/*
npm install class-variance-authority
npm install clsx tailwind-merge
npm install @grafana/scenes
npm install react-query
```

### Требования к системе

- Linux 5.8+ (для eBPF)
- Clang 14+ (для сборки eBPF и WASM)
- Docker (для изоляции сборки)
- PostgreSQL 15+
- Garage S3 (или совместимый)

### Риски

| Риск | Вероятность | Влияние | Митигация |
|------|-------------|---------|-----------|
| eBPF верификатор отклоняет программы | Средняя | Высокое | Тестирование на разных ядрах |
| WASM runtime производительность | Средняя | Среднее | Бенчмарки, оптимизация |
| Garage S3 нестабильность | Низкая | Среднее | Кэширование, retry logic |
| Grafana Scenes совместимость | Низкая | Низкое | Фоллбэк на простые графики |

---

## ✅ Критерии готовности

- [ ] Плагины загружаются из Git и собираются автоматически
- [ ] eBPF программы загружаются в ядро и работают
- [ ] WASM плагины выполняют обработку метрик
- [ ] Фильтры применяются к метрикам в реальном времени
- [ ] UI отображает плагины, метрики и дашборды
- [ ] `/metrics` endpoint отдаёт Prometheus-совместимый формат
- [ ] Система работает в rootless режиме
- [ ] Примеры плагинов работают корректно
- [ ] Документация полная и понятная
