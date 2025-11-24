# Обработка ошибок

Стратегия обработки ошибок в проекте: от доменных ошибок до HTTP ответов.

## Принципы

1. **Единообразие** - Все ошибки обрабатываются одинаково
2. **Типизация** - Используются типизированные ошибки домена
3. **Контекст** - Ошибки содержат достаточно информации для отладки
4. **Безопасность** - Внешние ошибки не раскрывают внутренние детали

## Иерархия ошибок

```
Domain Errors (domain/errors.go)
    ↓
Use Case преобразует ошибки БД в доменные
    ↓
Handler преобразует доменные ошибки в HTTP статусы
    ↓
HTTP Response с кодом ошибки
```

## Доменные ошибки

### Определение (`domain/errors.go`)

```go
type ErrorCode string

const (
    ErrCodeInvalid      ErrorCode = "INVALID"
    ErrCodeNotFound     ErrorCode = "NOT_FOUND"
    ErrCodeUnauthorized ErrorCode = "UNAUTHORIZED"
    ErrCodeForbidden    ErrorCode = "FORBIDDEN"
    ErrCodeInternal     ErrorCode = "INTERNAL_ERROR"
)

type DomainError struct {
    Code    ErrorCode
    Message string
    Details map[string]interface{}
}

func (e *DomainError) Error() string {
    return e.Message
}

// Предопределенные ошибки
var (
    ErrInvalidPayload = &DomainError{
        Code:    ErrCodeInvalid,
        Message: "invalid payload",
    }
    
    ErrUserNotFound = &DomainError{
        Code:    ErrCodeNotFound,
        Message: "user not found",
    }
    
    ErrTaskNotFound = &DomainError{
        Code:    ErrCodeNotFound,
        Message: "task not found",
    }
)
```

### Использование

```go
// В Use Case
func (uc *UseCase) GetTask(ctx context.Context, id string) (*domain.Task, error) {
    task, err := uc.repo.GetByID(ctx, id)
    if err != nil {
        // Преобразование ошибки БД в доменную
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, domain.ErrTaskNotFound
        }
        return nil, err
    }
    return task, nil
}
```

## Обработка в Repository

### Преобразование ошибок БД

```go
// repository/postgres/task_repo.go
func (r *taskRepository) GetByID(ctx context.Context, id string) (*domain.Task, error) {
    query := `SELECT ... FROM tasks WHERE id = $1`
    
    row := r.pool.QueryRow(ctx, query, id)
    task, err := scanTask(row)
    if err != nil {
        // Преобразование ошибки БД в доменную
        if errors.Is(err, pgx.ErrNoRows) {
            return nil, domain.ErrTaskNotFound
        }
        return nil, err
    }
    
    return task, nil
}
```

### Валидация данных

```go
func (r *taskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // Валидация на уровне репозитория
    if task == nil {
        return nil, domain.ErrInvalidPayload
    }
    
    if task.ID == "" {
        task.ID = uuid.NewString()
    }
    
    // SQL запрос...
}
```

## Обработка в Use Case

### Валидация бизнес-правил

```go
// usecase/task/task.go
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // Валидация обязательных полей
    if task.Title == "" {
        return nil, domain.ErrInvalidPayload
    }
    
    // Проверка бизнес-правил
    if task.Priority != "low" && task.Priority != "medium" && task.Priority != "high" {
        return nil, &domain.DomainError{
            Code:    domain.ErrCodeInvalid,
            Message: "invalid priority value",
        }
    }
    
    // Вызов репозитория
    created, err := uc.repo.Create(ctx, task)
    if err != nil {
        // Ошибка уже доменная (преобразована в репозитории)
        return nil, err
    }
    
    return created, nil
}
```

### Обработка ошибок с буферизацией

```go
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    created, err := uc.repo.Create(ctx, task)
    if err != nil {
        // Если БД недоступна, буферизуем операцию
        if isConnectionError(err) && uc.buffer != nil {
            item := buffer.Item{
                Entity:    usecase.EntityTask,
                Operation: usecase.OperationCreate,
                Data:      marshalTask(task),
            }
            if bufErr := uc.buffer.BufferOperation(ctx, item); bufErr == nil {
                // Операция буферизована, возвращаем успех
                return task, nil
            }
        }
        return nil, err
    }
    
    return created, nil
}
```

## Обработка в Handler

### Преобразование в HTTP статусы

```go
// api/handler/base.go
func mapError(err error) (int, string) {
    switch {
    case domain.IsDomainError(err, domain.ErrCodeUnauthorized):
        return http.StatusUnauthorized, string(domain.ErrCodeUnauthorized)
    case domain.IsDomainError(err, domain.ErrCodeForbidden):
        return http.StatusForbidden, string(domain.ErrCodeForbidden)
    case domain.IsDomainError(err, domain.ErrCodeInvalid):
        return http.StatusBadRequest, string(domain.ErrCodeInvalid)
    case domain.IsDomainError(err, domain.ErrCodeNotFound):
        return http.StatusNotFound, string(domain.ErrCodeNotFound)
    default:
        return http.StatusInternalServerError, string(domain.ErrCodeInternal)
    }
}

func (h baseHandler) respondError(ctx *fasthttp.RequestCtx, err error) {
    status, code := mapError(err)
    
    // Безопасное сообщение об ошибке
    message := err.Error()
    if status == http.StatusInternalServerError {
        // Не раскрываем внутренние детали
        message = "internal server error"
    }
    
    h.respondJSON(ctx, status, transport.NewError(code, message, nil))
}
```

### Использование в handlers

```go
// api/handler/task.go
func (h *TaskHandler) GetTask(ctx *fasthttp.RequestCtx) {
    taskID := ctx.UserValue("id").(string)
    
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()
    
    task, err := h.uc.GetTask(stdCtx, taskID)
    if err != nil {
        // Автоматическое преобразование в HTTP статус
        h.respondError(ctx, err)
        return
    }
    
    h.respondSuccess(ctx, http.StatusOK, task)
}
```

## Маппинг ошибок

### Таблица соответствия

| Доменная ошибка | HTTP статус | Код ошибки |
|----------------|-------------|------------|
| `ErrCodeInvalid` | 400 Bad Request | `INVALID` |
| `ErrCodeNotFound` | 404 Not Found | `NOT_FOUND` |
| `ErrCodeUnauthorized` | 401 Unauthorized | `UNAUTHORIZED` |
| `ErrCodeForbidden` | 403 Forbidden | `FORBIDDEN` |
| `ErrCodeInternal` | 500 Internal Server Error | `INTERNAL_ERROR` |

### Примеры ответов

#### Успешный ответ

```json
{
  "success": true,
  "data": {
    "id": "123",
    "title": "Задача"
  }
}
```

#### Ошибка валидации (400)

```json
{
  "success": false,
  "error": {
    "code": "INVALID",
    "message": "invalid payload"
  }
}
```

#### Не найдено (404)

```json
{
  "success": false,
  "error": {
    "code": "NOT_FOUND",
    "message": "task not found"
  }
}
```

#### Внутренняя ошибка (500)

```json
{
  "success": false,
  "error": {
    "code": "INTERNAL_ERROR",
    "message": "internal server error"
  }
}
```

## Логирование ошибок

### В Use Case

```go
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    created, err := uc.repo.Create(ctx, task)
    if err != nil {
        // Логируем ошибку с контекстом
        uc.logger.Error("failed to create task",
            zap.Error(err),
            zap.String("task_id", task.ID),
            zap.String("user_id", task.UserID),
        )
        return nil, err
    }
    
    return created, nil
}
```

### В Handler

```go
func (h *TaskHandler) CreateTask(ctx *fasthttp.RequestCtx) {
    // ...
    
    created, err := h.uc.CreateTask(stdCtx, task)
    if err != nil {
        // Логируем только внутренние ошибки
        if !domain.IsDomainError(err, domain.ErrCodeInvalid) &&
           !domain.IsDomainError(err, domain.ErrCodeNotFound) {
            h.logger.Error("failed to create task",
                zap.Error(err),
                zap.String("request_id", getRequestID(ctx)),
            )
        }
        
        h.respondError(ctx, err)
        return
    }
    
    h.respondSuccess(ctx, http.StatusCreated, created)
}
```

## Best Practices

### ✅ Хорошо

1. **Используйте доменные ошибки**:
```go
return nil, domain.ErrTaskNotFound
```

2. **Логируйте с контекстом**:
```go
logger.Error("operation failed", zap.Error(err), zap.String("id", id))
```

3. **Преобразуйте ошибки БД в доменные**:
```go
if errors.Is(err, pgx.ErrNoRows) {
    return nil, domain.ErrTaskNotFound
}
```

### ❌ Плохо

1. **Не возвращайте сырые ошибки БД**:
```go
// Плохо
return nil, err  // Раскрывает внутренние детали

// Хорошо
return nil, domain.ErrTaskNotFound
```

2. **Не логируйте пользовательские ошибки**:
```go
// Плохо
logger.Error("user not found")  // Это нормальная ситуация

// Хорошо
logger.Error("database error", zap.Error(err))  // Только реальные ошибки
```

3. **Не раскрывайте внутренние детали**:
```go
// Плохо
message := "connection to postgres://user:pass@host failed"

// Хорошо
message := "internal server error"
```

## Следующие шаги

- [Контекст и таймауты](./context.md)

