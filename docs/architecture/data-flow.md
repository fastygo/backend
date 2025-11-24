# Потоки данных

Как данные проходят через приложение от HTTP запроса до базы данных и обратно.

## Общий поток

```
HTTP Request
    ↓
Router (маршрутизация)
    ↓
Middleware (аутентификация, логирование)
    ↓
Handler (парсинг запроса)
    ↓
Use Case (бизнес-логика)
    ↓
Repository (доступ к данным)
    ↓
Database/Redis
    ↓
Response (обратно через все слои)
```

## Пример: создание задачи

Давайте проследим полный путь создания задачи от HTTP запроса до сохранения в БД.

### 1. HTTP Request

Клиент отправляет POST запрос:

```http
POST /api/v1/tasks HTTP/1.1
Host: localhost:8080
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
Content-Type: application/json

{
  "title": "Создать документацию",
  "description": "Написать подробную документацию для проекта",
  "priority": "high"
}
```

### 2. Router (`internal/router/router.go`)

Роутер определяет, какой handler обработает запрос:

```go
r.POST("/api/v1/tasks", authMiddleware(handlers.Task.CreateTask))
```

**Что происходит**:
- Роутер находит соответствие `/api/v1/tasks` → `handlers.Task.CreateTask`
- Применяет middleware для аутентификации

### 3. Middleware (`internal/middleware/auth.go`)

Middleware проверяет JWT токен:

```go
func JWTAuth(jwtSecret string) func(fasthttp.RequestHandler) fasthttp.RequestHandler {
    return func(next fasthttp.RequestHandler) fasthttp.RequestHandler {
        return func(ctx *fasthttp.RequestCtx) {
            // 1. Извлечение токена из заголовка
            token := extractToken(ctx)
            
            // 2. Валидация токена
            if !validateToken(token, jwtSecret) {
                ctx.SetStatusCode(fasthttp.StatusUnauthorized)
                return
            }
            
            // 3. Извлечение user_id из токена
            userID := extractUserID(token)
            ctx.Request.Header.Set("X-User-ID", userID)
            
            // 4. Передача управления следующему handler
            next(ctx)
        }
    }
}
```

**Результат**: В заголовке `X-User-ID` появляется ID пользователя.

### 4. Handler (`api/handler/task.go`)

Handler парсит запрос и вызывает Use Case:

```go
func (h *TaskHandler) CreateTask(ctx *fasthttp.RequestCtx) {
    // 1. Извлечение user_id из заголовка (установлен middleware)
    userID := string(ctx.Request.Header.Peek("X-User-ID"))
    if userID == "" {
        h.respondJSON(ctx, http.StatusUnauthorized, ...)
        return
    }
    
    // 2. Парсинг JSON тела запроса
    var req transport.TaskRequest
    if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
        h.respondJSON(ctx, http.StatusBadRequest, ...)
        return
    }
    
    // 3. Преобразование DTO в доменную сущность
    task := &domain.Task{
        UserID:      userID,
        Title:       req.Title,
        Description: req.Description,
        Priority:    req.Priority,
    }
    
    // 4. Получение контекста с таймаутом
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()
    
    // 5. Вызов Use Case
    created, err := h.uc.CreateTask(stdCtx, task)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    // 6. Отправка успешного ответа
    h.respondSuccess(ctx, http.StatusCreated, created)
}
```

**Что происходит**:
- Парсинг HTTP запроса в DTO
- Преобразование DTO в доменную сущность
- Вызов Use Case
- Форматирование ответа

### 5. Use Case (`usecase/task/task.go`)

Use Case применяет бизнес-логику:

```go
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // 1. Валидация данных
    if task.Title == "" {
        return nil, domain.ErrInvalidPayload
    }
    
    // 2. Применение бизнес-правил
    if task.Status == "" {
        task.Status = "pending"  // Значение по умолчанию
    }
    
    // 3. Генерация ID (если не указан)
    if task.ID == "" {
        task.ID = uuid.NewString()
    }
    
    // 4. Сохранение через репозиторий
    created, err := uc.repo.Create(ctx, task)
    if err != nil {
        // 5. Если БД недоступна, буферизуем операцию
        if uc.buffer != nil {
            item := buffer.Item{
                Entity:    usecase.EntityTask,
                Operation: usecase.OperationCreate,
                Data:      marshalTask(task),
            }
            uc.buffer.BufferOperation(ctx, item)
        }
        return nil, err
    }
    
    return created, nil
}
```

**Что происходит**:
- Валидация и применение бизнес-правил
- Вызов репозитория для сохранения
- Обработка ошибок с буферизацией при необходимости

### 6. Repository (`repository/postgres/task_repo.go`)

Repository сохраняет данные в БД:

```go
func (r *taskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // 1. SQL запрос
    query := `
        INSERT INTO tasks (id, user_id, title, description, status, priority, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
        RETURNING created_at, updated_at
    `
    
    // 2. Выполнение запроса
    err := r.pool.QueryRow(ctx, query,
        task.ID,
        task.UserID,
        task.Title,
        task.Description,
        task.Status,
        task.Priority,
    ).Scan(&task.CreatedAt, &task.UpdatedAt)
    
    if err != nil {
        return nil, err
    }
    
    return task, nil
}
```

**Что происходит**:
- Формирование SQL запроса
- Выполнение запроса с параметрами
- Маппинг результата обратно в доменную сущность

### 7. Database

PostgreSQL сохраняет данные:

```sql
INSERT INTO tasks (id, user_id, title, description, status, priority, created_at, updated_at)
VALUES ('550e8400-e29b-41d4-a716-446655440000', 'user-123', 'Создать документацию', 
        'Написать подробную документацию для проекта', 'pending', 'high', 
        '2024-01-15 10:30:00', '2024-01-15 10:30:00')
RETURNING created_at, updated_at;
```

### 8. Response (обратный путь)

Данные проходят обратно через все слои:

```
Database → Repository → Use Case → Handler → HTTP Response
```

**HTTP Response**:

```http
HTTP/1.1 201 Created
Content-Type: application/json

{
  "success": true,
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "user_id": "user-123",
    "title": "Создать документацию",
    "description": "Написать подробную документацию для проекта",
    "status": "pending",
    "priority": "high",
    "created_at": "2024-01-15T10:30:00Z",
    "updated_at": "2024-01-15T10:30:00Z"
  }
}
```

## Поток чтения данных

Пример: получение списка задач

```
HTTP GET /api/v1/tasks
    ↓
Router → Handler.GetTasks
    ↓
Handler парсит query параметры (limit, offset, status)
    ↓
Use Case.ListTasks (может применять фильтры, сортировку)
    ↓
Repository.List (SQL запрос с WHERE и LIMIT)
    ↓
Database (SELECT ... WHERE ... LIMIT ...)
    ↓
Repository маппит строки БД в []domain.Task
    ↓
Use Case возвращает список
    ↓
Handler сериализует в JSON
    ↓
HTTP Response
```

## Поток с ошибкой

Что происходит, если что-то пошло не так:

```
Database Error (например, connection timeout)
    ↓
Repository возвращает error
    ↓
Use Case проверяет ошибку:
    - Если БД недоступна → буферизует операцию
    - Если другая ошибка → возвращает доменную ошибку
    ↓
Handler получает error
    ↓
Handler.mapError преобразует доменную ошибку в HTTP статус
    ↓
HTTP Response с ошибкой:
{
  "success": false,
  "error": {
    "code": "INTERNAL_ERROR",
    "message": "database connection failed"
  }
}
```

## Offline Resilience Flow

Что происходит, когда БД недоступна:

```
1. Repository.Create возвращает error (БД недоступна)
    ↓
2. Use Case обнаруживает ошибку
    ↓
3. Use Case вызывает buffer.BufferOperation
    ↓
4. Операция сохраняется в BoltDB (локальный файл)
    ↓
5. Handler возвращает успешный ответ клиенту (операция принята)
    ↓
6. Background процесс (BufferProcessor) периодически проверяет:
    - Доступна ли БД?
    - Есть ли операции в буфере?
    ↓
7. Когда БД становится доступной:
    - BufferProcessor читает операции из буфера
    - Выполняет их через Repository
    - Удаляет из буфера после успешного выполнения
```

## Следующие шаги

- [Обработка ошибок](./error-handling.md)
- [Контекст и таймауты](./context.md)

