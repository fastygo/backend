# Слои приложения

В этом разделе подробно описаны все слои приложения и их ответственность.

## Domain Layer (Доменный слой)

**Путь**: `domain/`

### Назначение

Доменный слой содержит бизнес-сущности и правила. Это ядро приложения, которое не зависит ни от чего внешнего.

### Что содержит

#### Сущности (Entities)

Сущности представляют основные бизнес-объекты:

```go
// domain/user.go
type User struct {
    ID        string            `json:"id"`
    Email     string            `json:"email"`
    Role      string            `json:"role"`
    Status    string            `json:"status"`
    Metadata  map[string]string `json:"metadata,omitempty"`
    CreatedAt time.Time         `json:"created_at"`
    UpdatedAt time.Time         `json:"updated_at"`
}
```

**Особенности**:
- Содержат только данные и методы для работы с ними
- Не знают о БД, HTTP или других внешних вещах
- Могут содержать бизнес-логику (методы валидации)

#### Ошибки домена

```go
// domain/errors.go
var (
    ErrUserNotFound = &DomainError{
        Code:    ErrCodeNotFound,
        Message: "user not found",
    }
)
```

**Зачем**: Единообразная обработка ошибок во всем приложении.

### Примеры файлов

- `domain/user.go` - Сущность пользователя
- `domain/task.go` - Сущность задачи
- `domain/session.go` - Сущность сессии
- `domain/errors.go` - Ошибки домена
- `domain/aggregate.go` - Универсальная сущность для расширения

### Правила

✅ **Можно**:
- Определять структуры данных
- Добавлять методы валидации
- Определять бизнес-правила
- Создавать типы и константы

❌ **Нельзя**:
- Импортировать пакеты из `repository/`, `usecase/`, `api/`
- Использовать SQL или HTTP библиотеки
- Зависеть от внешних сервисов

---

## Use Case Layer (Слой бизнес-логики)

**Путь**: `usecase/`

### Назначение

Use Case содержит бизнес-логику приложения. Это слой, который координирует работу репозиториев и применяет бизнес-правила.

### Структура

```
usecase/
├── auth/
│   └── auth.go          # Аутентификация и сессии
├── profile/
│   └── profile.go      # Профиль пользователя
├── task/
│   └── task.go         # Задачи
├── buffer_port.go       # Интерфейс для буферизации
├── constants.go         # Константы
└── dispatcher.go       # Диспетчер для расширения
```

### Пример Use Case

```go
// usecase/task/task.go
type UseCase struct {
    repo   repository.TaskRepository
    buffer usecase.BufferPort
    logger *zap.Logger
}

func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // 1. Валидация
    if task.Title == "" {
        return nil, domain.ErrInvalidPayload
    }
    
    // 2. Применение бизнес-правил
    if task.Status == "" {
        task.Status = "pending"
    }
    
    // 3. Сохранение через репозиторий
    created, err := uc.repo.Create(ctx, task)
    if err != nil {
        // 4. Буферизация при ошибке (offline resilience)
        if uc.buffer != nil {
            uc.buffer.BufferOperation(...)
        }
        return nil, err
    }
    
    return created, nil
}
```

### Ответственность

1. **Валидация данных** - Проверка входных данных
2. **Бизнес-логика** - Применение правил бизнеса
3. **Оркестрация** - Координация вызовов репозиториев
4. **Обработка ошибок** - Преобразование ошибок БД в доменные ошибки
5. **Offline Resilience** - Буферизация операций при недоступности БД

### Правила

✅ **Можно**:
- Импортировать `domain/` и `repository/` (только интерфейсы)
- Содержать бизнес-логику
- Вызывать несколько репозиториев
- Использовать буфер для offline операций

❌ **Нельзя**:
- Импортировать `api/` или `internal/infrastructure/`
- Знать о HTTP или SQL
- Содержать код работы с БД напрямую

---

## Repository Layer (Слой доступа к данным)

**Путь**: `repository/`

### Назначение

Repository абстрагирует доступ к данным. Позволяет изменить реализацию БД без изменения бизнес-логики.

### Структура

```
repository/
├── user.go              # Интерфейс UserRepository
├── task.go              # Интерфейс TaskRepository
├── session.go           # Интерфейс SessionRepository
├── aggregate.go         # Интерфейс AggregateRepository
├── postgres/
│   ├── user_repo.go     # Реализация для PostgreSQL
│   ├── task_repo.go
│   └── helpers.go       # Вспомогательные функции
└── redis/
    └── session_repo.go  # Реализация для Redis
```

### Интерфейс

```go
// repository/task.go
type TaskRepository interface {
    GetByID(ctx context.Context, id string) (*domain.Task, error)
    List(ctx context.Context, filter TaskFilter) ([]domain.Task, error)
    Create(ctx context.Context, task *domain.Task) (*domain.Task, error)
    Update(ctx context.Context, task *domain.Task) error
    Delete(ctx context.Context, id string) error
}
```

### Реализация

```go
// repository/postgres/task_repo.go
type taskRepository struct {
    pool *pgxpool.Pool
}

func (r *taskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    query := `
        INSERT INTO tasks (id, user_id, title, description, status, priority)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING created_at, updated_at
    `
    
    err := r.pool.QueryRow(ctx, query,
        task.ID, task.UserID, task.Title,
        task.Description, task.Status, task.Priority,
    ).Scan(&task.CreatedAt, &task.UpdatedAt)
    
    if err != nil {
        return nil, err
    }
    
    return task, nil
}
```

### Ответственность

1. **Маппинг данных** - Преобразование между БД и доменными сущностями
2. **SQL запросы** - Выполнение запросов к БД
3. **Обработка ошибок БД** - Преобразование в доменные ошибки
4. **Транзакции** - Управление транзакциями (если нужно)

### Правила

✅ **Можно**:
- Импортировать `domain/` и библиотеки БД (pgx, redis)
- Содержать SQL запросы
- Маппить данные между БД и доменом

❌ **Нельзя**:
- Содержать бизнес-логику
- Импортировать `usecase/` или `api/`
- Знать о HTTP

---

## API Layer (HTTP слой)

**Путь**: `api/`

### Назначение

API слой обрабатывает HTTP запросы и ответы. Это тонкий слой, который делегирует работу Use Case.

### Структура

```
api/
├── handler/
│   ├── base.go         # Базовый handler с общими методами
│   ├── auth.go         # Аутентификация
│   ├── profile.go      # Профиль
│   ├── task.go         # Задачи
│   └── health.go       # Health check
└── transport/
    ├── request.go      # DTO для запросов
    └── response.go     # DTO для ответов
```

### Пример Handler

```go
// api/handler/task.go
type TaskHandler struct {
    baseHandler
    uc *taskUC.UseCase
}

func (h *TaskHandler) CreateTask(ctx *fasthttp.RequestCtx) {
    // 1. Парсинг HTTP запроса
    task, ok := h.parseTask(ctx, userID)
    if !ok {
        return
    }
    
    // 2. Получение контекста с таймаутом
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()
    
    // 3. Вызов Use Case
    created, err := h.uc.CreateTask(stdCtx, task)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    // 4. Отправка ответа
    h.respondSuccess(ctx, http.StatusCreated, created)
}
```

### Ответственность

1. **Парсинг запросов** - Извлечение данных из HTTP запроса
2. **Валидация входных данных** - Проверка формата данных
3. **Вызов Use Case** - Делегирование бизнес-логики
4. **Форматирование ответов** - Сериализация в JSON
5. **Обработка ошибок** - Преобразование ошибок в HTTP статусы

### Правила

✅ **Можно**:
- Импортировать `usecase/`, `domain/`, `api/transport/`
- Работать с HTTP (fasthttp)
- Парсить и сериализовать JSON

❌ **Нельзя**:
- Содержать бизнес-логику
- Импортировать `repository/` напрямую
- Знать о БД или SQL

---

## Infrastructure Layer (Инфраструктурный слой)

**Путь**: `internal/infrastructure/`

### Назначение

Инфраструктурный слой содержит подключения к внешним сервисам и технические детали.

### Структура

```
internal/infrastructure/
├── postgres/
│   ├── client.go      # Подключение к PostgreSQL
│   └── migrate.go     # Миграции БД
├── redis/
│   └── client.go      # Подключение к Redis
├── buffer/
│   ├── boltdb.go      # BoltDB для буферизации
│   └── types.go       # Типы для буфера
└── monitor/
    ├── connection.go  # Мониторинг соединений
    └── status.go      # Статус соединений
```

### Примеры

#### PostgreSQL Client

```go
// internal/infrastructure/postgres/client.go
func NewClient(cfg config.PostgresConfig) (*pgxpool.Pool, error) {
    connString := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s",
        cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Name,
    )
    
    pool, err := pgxpool.New(context.Background(), connString)
    if err != nil {
        return nil, err
    }
    
    // Проверка соединения
    if err := pool.Ping(context.Background()); err != nil {
        return nil, err
    }
    
    return pool, nil
}
```

#### Connection Monitor

```go
// internal/infrastructure/monitor/connection.go
type Monitor struct {
    pg     *pgxpool.Pool
    redis  *redis.Client
    buffer *buffer.Store
    status Status
}

func (m *Monitor) IsOnline() bool {
    return m.status.PostgreSQL && m.status.Redis
}
```

### Ответственность

1. **Подключения** - Инициализация соединений с БД, Redis и т.д.
2. **Мониторинг** - Отслеживание состояния соединений
3. **Миграции** - Управление схемой БД
4. **Буферизация** - Offline операции через BoltDB

---

## Middleware Layer

**Путь**: `internal/middleware/`

### Назначение

Middleware обрабатывает запросы до того, как они попадут в handler.

### Примеры

#### JWT Authentication

```go
// internal/middleware/auth.go
func JWTAuth(jwtSecret string) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
    return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
        return func(ctx *fasthttp.RequestCtx) {
            token := extractToken(ctx)
            if !validateToken(token, jwtSecret) {
                ctx.SetStatusCode(fasthttp.StatusUnauthorized)
                return
            }
            
            // Извлечение user_id из токена
            userID := extractUserID(token)
            ctx.Request.Header.Set("X-User-ID", userID)
            
            next(ctx)
        }
    }
}
```

### Ответственность

1. **Аутентификация** - Проверка JWT токенов
2. **Логирование** - Логирование запросов
3. **CORS** - Настройка CORS заголовков
4. **Request ID** - Генерация уникальных ID для запросов

---

## Следующие шаги

- [Структура директорий](./directory-structure.md)
- [Потоки данных](./data-flow.md)
- [Обработка ошибок](./error-handling.md)

