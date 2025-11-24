# Структура директорий

Подробное описание организации файлов и папок в проекте.

## Общая структура

```
backend/
├── cmd/                    # Точки входа приложения
├── internal/              # Внутренний код (не импортируется извне)
├── api/                   # HTTP интерфейс
├── usecase/               # Бизнес-логика
├── repository/            # Доступ к данным
├── domain/                # Доменные сущности
├── pkg/                   # Переиспользуемые пакеты
├── assets/                # Статические ресурсы
├── docs/                  # Документация
├── Dockerfile             # Docker образ
├── docker-compose.yml     # Docker Compose для разработки
├── Makefile               # Команды для разработки
├── go.mod                 # Go модуль
└── .env.example           # Пример конфигурации
```

## Детальное описание

### cmd/

**Назначение**: Точки входа приложения

```
cmd/
└── server/
    └── main.go           # Главная функция, инициализация всех компонентов
```

**Что делает `main.go`**:
1. Загружает конфигурацию
2. Инициализирует подключения (PostgreSQL, Redis, BoltDB)
3. Создает репозитории, use cases, handlers
4. Настраивает роутер и middleware
5. Запускает HTTP сервер
6. Обрабатывает graceful shutdown

### internal/

**Назначение**: Внутренний код, который не должен импортироваться из других модулей

```
internal/
├── config/
│   └── config.go        # Загрузка конфигурации из переменных окружения
├── infrastructure/
│   ├── postgres/
│   │   ├── client.go     # Подключение к PostgreSQL
│   │   └── migrate.go    # Миграции БД
│   ├── redis/
│   │   └── client.go     # Подключение к Redis
│   ├── buffer/
│   │   ├── boltdb.go     # BoltDB для буферизации
│   │   └── types.go      # Типы для буфера
│   └── monitor/
│       ├── connection.go # Мониторинг соединений
│       └── status.go     # Статус соединений
├── middleware/
│   └── auth.go          # JWT аутентификация
├── router/
│   └── router.go        # Настройка маршрутов
└── services/
    ├── buffer_processor.go  # Обработка буфера
    ├── buffer_bridge.go     # Мост между usecase и буфером
    └── lifecycle/
        └── manager.go       # Управление жизненным циклом
```

### api/

**Назначение**: HTTP интерфейс приложения

```
api/
├── handler/
│   ├── base.go          # Базовый handler с общими методами
│   ├── auth.go          # POST /api/v1/auth/login, /api/v1/auth/refresh
│   ├── profile.go       # GET /api/v1/profile, PUT /api/v1/profile
│   ├── task.go          # CRUD операции для задач
│   └── health.go         # GET /health
└── transport/
    ├── request.go       # DTO для запросов (AuthLoginRequest, TaskRequest и т.д.)
    └── response.go      # DTO для ответов (Envelope, Success, Error)
```

**Правила**:
- Handlers только парсят запросы и вызывают Use Cases
- Transport содержит структуры для сериализации JSON
- Не содержит бизнес-логику

### usecase/

**Назначение**: Бизнес-логика приложения

```
usecase/
├── auth/
│   └── auth.go         # Создание и обновление сессий
├── profile/
│   └── profile.go      # Получение и обновление профиля
├── task/
│   └── task.go         # CRUD операции для задач
├── buffer_port.go      # Интерфейс для буферизации операций
├── constants.go        # Константы (EntityTask, OperationCreate и т.д.)
└── dispatcher.go       # Диспетчер для расширения функциональности
```

**Правила**:
- Каждый use case в своей папке
- Использует только интерфейсы репозиториев
- Содержит всю бизнес-логику
- Может использовать буфер для offline операций

### repository/

**Назначение**: Доступ к данным

```
repository/
├── user.go             # Интерфейс UserRepository
├── task.go             # Интерфейс TaskRepository
├── session.go          # Интерфейс SessionRepository
├── aggregate.go        # Интерфейс AggregateRepository (для расширения)
├── postgres/
│   ├── user_repo.go   # Реализация UserRepository для PostgreSQL
│   ├── task_repo.go   # Реализация TaskRepository для PostgreSQL
│   ├── aggregate_repo.go  # Реализация AggregateRepository
│   └── helpers.go     # Вспомогательные функции (scanUser, marshalMap и т.д.)
└── redis/
    └── session_repo.go # Реализация SessionRepository для Redis
```

**Правила**:
- Интерфейсы в корне `repository/`
- Реализации в подпапках (`postgres/`, `redis/`)
- Легко заменить реализацию (например, на MongoDB)

### domain/

**Назначение**: Доменные сущности и бизнес-правила

```
domain/
├── user.go            # Сущность User
├── task.go            # Сущность Task
├── session.go         # Сущность Session
├── aggregate.go       # Универсальная сущность Aggregate (для расширения)
├── errors.go          # Доменные ошибки
└── types.go           # Общие типы (если нужно)
```

**Правила**:
- Только структуры данных и методы для работы с ними
- Не зависит от внешних библиотек (кроме стандартных)
- Содержит бизнес-правила

### pkg/

**Назначение**: Переиспользуемые пакеты, которые могут импортироваться извне

```
pkg/
├── logger/
│   └── logger.go      # Настройка Zap логгера
└── httpcontext/
    └── adapter.go     # Адаптер для преобразования fasthttp.RequestCtx в context.Context
```

**Правила**:
- Публичные API (начинаются с заглавной буквы)
- Могут использоваться в других проектах
- Не содержат бизнес-логику

### assets/

**Назначение**: Статические ресурсы

```
assets/
├── migrations/
│   ├── 001_initial.sql      # Первоначальная схема БД
│   ├── 002_add_indexes.sql  # Индексы
│   └── 003_update_schema.sql # Обновления схемы
└── fixtures/
    └── test_data.sql        # Тестовые данные
```

## Соглашения об именовании

### Файлы

- **Сущности**: `user.go`, `task.go` (существительное в единственном числе)
- **Use Cases**: `{usecase}.go` (например, `profile.go`, `auth.go`)
- **Repositories**: `{entity}_repo.go` (например, `user_repo.go`)
- **Handlers**: `{resource}.go` (например, `profile.go`, `task.go`)
- **Tests**: `{filename}_test.go`

### Пакеты

- **Имена пакетов**: в нижнем регистре, одно слово
- **Импорты**: используют алиасы для избежания конфликтов
  ```go
  import (
      apiHandler "github.com/fastygo/backend/api/handler"
      pgInfra "github.com/fastygo/backend/internal/infrastructure/postgres"
  )
  ```

### Структуры и функции

- **Публичные**: начинаются с заглавной буквы (`User`, `NewUserRepository`)
- **Приватные**: начинаются со строчной буквы (`user`, `newUserRepository`)
- **Конструкторы**: `New{Type}` (например, `NewUserRepository`)

## Примеры организации

### Добавление новой сущности "Project"

1. **Domain** (`domain/project.go`):
```go
package domain

type Project struct {
    ID          string
    Name        string
    Description string
    // ...
}
```

2. **Repository Interface** (`repository/project.go`):
```go
package repository

type ProjectRepository interface {
    GetByID(ctx context.Context, id string) (*domain.Project, error)
    // ...
}
```

3. **Repository Implementation** (`repository/postgres/project_repo.go`):
```go
package postgres

type projectRepository struct {
    pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) repository.ProjectRepository {
    return &projectRepository{pool: pool}
}
```

4. **Use Case** (`usecase/project/project.go`):
```go
package project

type UseCase struct {
    repo repository.ProjectRepository
}

func NewUseCase(repo repository.ProjectRepository) *UseCase {
    return &UseCase{repo: repo}
}
```

5. **Handler** (`api/handler/project.go`):
```go
package handler

type ProjectHandler struct {
    baseHandler
    uc *projectUC.UseCase
}
```

6. **Router** (`internal/router/router.go`):
```go
r.GET("/api/v1/projects", authMiddleware(handlers.Project.GetProjects))
```

## Следующие шаги

- [Потоки данных](./data-flow.md)
- [Обработка ошибок](./error-handling.md)
- [Контекст и таймауты](./context.md)

