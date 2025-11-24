# Контекст и таймауты

Работа с контекстами в проекте, особенно важная для fasthttp, который использует свой собственный контекст.

## Проблема

Fasthttp использует `*fasthttp.RequestCtx`, который **не совместим** со стандартным `context.Context`, требуемым для работы с PostgreSQL (pgx) и другими библиотеками.

## Решение: Context Adapter

Мы создали адаптер, который преобразует `fasthttp.RequestCtx` в стандартный `context.Context` с таймаутами.

### Реализация (`pkg/httpcontext/adapter.go`)

```go
package httpcontext

import (
    "context"
    "time"
    
    "github.com/google/uuid"
    "github.com/valyala/fasthttp"
    
    appLogger "github.com/fastygo/backend/pkg/logger"
)

type Adapter struct {
    timeout time.Duration
    logger  *zap.Logger
}

func NewAdapter(timeout time.Duration, logger *zap.Logger) *Adapter {
    if timeout <= 0 {
        timeout = 30 * time.Second  // По умолчанию 30 секунд
    }
    return &Adapter{
        timeout: timeout,
        logger:  logger,
    }
}

// Attach создает context.Context из fasthttp.RequestCtx
func (a *Adapter) Attach(ctx *fasthttp.RequestCtx) (context.Context, context.CancelFunc) {
    // 1. Создаем базовый контекст
    stdCtx := context.Background()
    
    // 2. Добавляем Request ID для трейсинга
    requestID := a.getOrCreateRequestID(ctx)
    stdCtx = context.WithValue(stdCtx, "request_id", requestID)
    
    // 3. Добавляем таймаут
    stdCtx, cancel := context.WithTimeout(stdCtx, a.timeout)
    
    return stdCtx, cancel
}

func (a *Adapter) getOrCreateRequestID(ctx *fasthttp.RequestCtx) string {
    // Проверяем, есть ли уже Request ID в заголовке
    if id := ctx.Request.Header.Peek("X-Request-ID"); len(id) > 0 {
        return string(id)
    }
    
    // Создаем новый Request ID
    id := uuid.NewString()
    ctx.Request.Header.Set("X-Request-ID", id)
    ctx.Response.Header.Set("X-Request-ID", id)
    
    return id
}
```

## Использование в Handlers

### Базовый handler (`api/handler/base.go`)

```go
type baseHandler struct {
    adapter *httpcontext.Adapter
    logger  *zap.Logger
}

func (h baseHandler) requestContext(ctx *fasthttp.RequestCtx) (context.Context, context.CancelFunc) {
    if h.adapter != nil {
        return h.adapter.Attach(ctx)
    }
    // Fallback если адаптер не настроен
    return context.WithCancel(context.Background())
}
```

### Использование в конкретном handler

```go
// api/handler/task.go
func (h *TaskHandler) CreateTask(ctx *fasthttp.RequestCtx) {
    // Получаем стандартный context с таймаутом
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()  // Важно: отменяем контекст при выходе
    
    // Используем stdCtx для вызова Use Case
    created, err := h.uc.CreateTask(stdCtx, task)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    h.respondSuccess(ctx, http.StatusCreated, created)
}
```

## Таймауты

### Настройка таймаутов

Таймауты настраиваются в конфигурации:

```go
// internal/config/config.go
type ContextConfig struct {
    RequestTimeout   time.Duration `env:"REQUEST_TIMEOUT" envDefault:"30s"`
    ShutdownTimeout  time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"10s"`
}
```

### Зачем нужны таймауты?

1. **Защита от зависших запросов** - Запрос не может висеть бесконечно
2. **Освобождение ресурсов** - Контекст отменяется, соединения закрываются
3. **Улучшение UX** - Клиент получает ответ в разумное время

### Пример работы таймаута

```go
// Use Case получает контекст с таймаутом 30 секунд
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // Если операция занимает больше 30 секунд, контекст отменяется
    created, err := uc.repo.Create(ctx, task)
    if err != nil {
        // Проверяем, не истек ли таймаут
        if ctx.Err() == context.DeadlineExceeded {
            return nil, &domain.DomainError{
                Code:    domain.ErrCodeInternal,
                Message: "request timeout",
            }
        }
        return nil, err
    }
    
    return created, nil
}
```

## Request ID для трейсинга

### Что такое Request ID?

Request ID - это уникальный идентификатор каждого запроса, который помогает отслеживать запрос через все слои приложения.

### Как это работает?

1. **Генерация** - Создается при получении запроса (или берется из заголовка)
2. **Пропагация** - Передается через все слои через контекст
3. **Логирование** - Добавляется в каждую запись лога
4. **Ответ** - Возвращается клиенту в заголовке `X-Request-ID`

### Использование в логах

```go
// В Use Case
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // Извлекаем Request ID из контекста
    requestID := ctx.Value("request_id").(string)
    
    uc.logger.Info("creating task",
        zap.String("request_id", requestID),
        zap.String("task_id", task.ID),
    )
    
    // ...
}
```

### Пример логов с Request ID

```
2024-01-15T10:30:00Z INFO creating task request_id=550e8400-e29b-41d4-a716-446655440000 task_id=123
2024-01-15T10:30:01Z INFO task created request_id=550e8400-e29b-41d4-a716-446655440000 task_id=123
```

Теперь можно легко найти все логи для конкретного запроса!

## Отмена контекста

### Важно: всегда отменяйте контекст

```go
func (h *TaskHandler) CreateTask(ctx *fasthttp.RequestCtx) {
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()  // ← Обязательно!
    
    // ...
}
```

**Почему это важно?**
- Освобождает ресурсы (горутины, соединения)
- Предотвращает утечки памяти
- Корректно обрабатывает отмену операций

### Что происходит при отмене?

```go
// Repository получает отмененный контекст
func (r *taskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // pgx проверяет контекст перед выполнением запроса
    err := r.pool.QueryRow(ctx, query, ...).Scan(...)
    
    // Если контекст отменен, pgx вернет context.Canceled
    if err == context.Canceled {
        return nil, err
    }
    
    return task, nil
}
```

## Best Practices

### ✅ Хорошо

1. **Всегда используйте defer cancel()**:
```go
stdCtx, cancel := h.requestContext(ctx)
defer cancel()
```

2. **Проверяйте контекст в длительных операциях**:
```go
for _, item := range items {
    if ctx.Err() != nil {
        return ctx.Err()
    }
    process(item)
}
```

3. **Передавайте контекст во все вызовы**:
```go
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // Передаем ctx в репозиторий
    return uc.repo.Create(ctx, task)
}
```

### ❌ Плохо

1. **Не создавайте context.Background() в handlers**:
```go
// Плохо
ctx := context.Background()
uc.CreateTask(ctx, task)

// Хорошо
stdCtx, cancel := h.requestContext(ctx)
defer cancel()
uc.CreateTask(stdCtx, task)
```

2. **Не забывайте отменять контекст**:
```go
// Плохо
stdCtx, cancel := h.requestContext(ctx)
uc.CreateTask(stdCtx, task)  // cancel() не вызван!

// Хорошо
stdCtx, cancel := h.requestContext(ctx)
defer cancel()
uc.CreateTask(stdCtx, task)
```

3. **Не передавайте nil контекст**:
```go
// Плохо
uc.CreateTask(nil, task)

// Хорошо
uc.CreateTask(ctx, task)
```

## Следующие шаги

- [Разработка](../development/README.md)
- [Примеры использования](../examples/README.md)

