# Обзор архитектуры

## Что такое Clean Architecture?

Clean Architecture (Чистая архитектура) - это подход к проектированию программного обеспечения, который разделяет приложение на слои с четкими границами и правилами зависимостей.

## Зачем это нужно?

### Проблема монолитного кода

Представьте код, где все смешано:

```go
// Плохо: все в одном месте
func handler(w http.ResponseWriter, r *http.Request) {
    // Парсинг HTTP запроса
    // Валидация данных
    // Бизнес-логика
    // SQL запросы
    // Форматирование ответа
}
```

**Проблемы**:
- Сложно тестировать
- Сложно изменять
- Сложно понимать
- Невозможно переиспользовать

### Решение: разделение на слои

```go
// Хорошо: каждый слой отвечает за свое
func (h *Handler) CreateTask(ctx *fasthttp.RequestCtx) {
    // 1. Парсинг запроса (API слой)
    task := parseRequest(ctx)
    
    // 2. Бизнес-логика (UseCase слой)
    result, err := h.useCase.CreateTask(ctx, task)
    
    // 3. Ответ (API слой)
    h.respondSuccess(ctx, result)
}
```

## Структура проекта

```
backend/
├── cmd/server/          # Точка входа приложения
├── internal/            # Внутренний код (не импортируется извне)
│   ├── config/         # Конфигурация
│   ├── infrastructure/  # Подключения к БД, Redis и т.д.
│   ├── middleware/      # HTTP middleware
│   └── router/         # Маршрутизация
├── api/                 # HTTP интерфейс
│   ├── handler/        # HTTP обработчики
│   └── transport/      # DTO для запросов/ответов
├── usecase/            # Бизнес-логика
├── repository/         # Доступ к данным
│   ├── postgres/      # Реализация для PostgreSQL
│   └── redis/         # Реализация для Redis
├── domain/            # Доменные сущности и правила
└── pkg/               # Переиспользуемые пакеты
    ├── logger/       # Логирование
    └── httpcontext/  # Адаптер контекста
```

## Поток запроса

```
1. HTTP Request
   ↓
2. Router (маршрутизация)
   ↓
3. Middleware (аутентификация, логирование)
   ↓
4. Handler (парсинг запроса)
   ↓
5. UseCase (бизнес-логика)
   ↓
6. Repository (доступ к данным)
   ↓
7. Database/Redis
   ↓
8. Response (обратно через все слои)
```

## Пример: создание задачи

Давайте проследим, как создается задача:

### 1. HTTP Handler (`api/handler/task.go`)

```go
func (h *TaskHandler) CreateTask(ctx *fasthttp.RequestCtx) {
    // Парсим HTTP запрос
    task, ok := h.parseTask(ctx, userID)
    if !ok {
        return
    }
    
    // Вызываем UseCase
    created, err := h.uc.CreateTask(stdCtx, task)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    // Отправляем ответ
    h.respondSuccess(ctx, http.StatusCreated, created)
}
```

**Что делает**: Только HTTP обработка, никакой бизнес-логики.

### 2. Use Case (`usecase/task/task.go`)

```go
func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    // Валидация
    if task.Title == "" {
        return nil, domain.ErrInvalidPayload
    }
    
    // Бизнес-логика
    if task.Status == "" {
        task.Status = "pending"
    }
    
    // Сохранение через репозиторий
    created, err := uc.repo.Create(ctx, task)
    if err != nil {
        // Если БД недоступна, буферизуем операцию
        if uc.buffer != nil {
            uc.buffer.BufferOperation(...)
        }
        return nil, err
    }
    
    return created, nil
}
```

**Что делает**: Бизнес-логика и оркестрация.

### 3. Repository (`repository/postgres/task_repo.go`)

```go
func (r *taskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
    query := `INSERT INTO tasks (...) VALUES (...) RETURNING ...`
    
    err := r.pool.QueryRow(ctx, query, ...).Scan(...)
    if err != nil {
        return nil, err
    }
    
    return task, nil
}
```

**Что делает**: Только работа с БД, никакой бизнес-логики.

## Преимущества

### 1. Тестируемость

Каждый слой тестируется отдельно:

```go
// Тест UseCase с моком репозитория
func TestCreateTask(t *testing.T) {
    mockRepo := &MockTaskRepository{}
    uc := NewUseCase(mockRepo)
    
    task := &domain.Task{Title: "Test"}
    result, err := uc.CreateTask(ctx, task)
    
    assert.NoError(t, err)
    assert.Equal(t, "pending", result.Status)
}
```

### 2. Изменяемость

Можно изменить реализацию без изменения интерфейса:

```go
// Заменить PostgreSQL на MongoDB - изменить только repository/postgres/
// UseCase и Handler остаются без изменений
```

### 3. Понятность

Каждый файл имеет четкую ответственность:

- `handler/task.go` - HTTP обработка задач
- `usecase/task/task.go` - Бизнес-логика задач
- `repository/postgres/task_repo.go` - SQL запросы для задач

## Принципы проектирования

### Dependency Inversion (Инверсия зависимостей)

Высокоуровневые модули не зависят от низкоуровневых. Оба зависят от абстракций.

```go
// UseCase зависит от интерфейса, а не от конкретной реализации
type TaskRepository interface {
    Create(ctx context.Context, task *Task) (*Task, error)
}

// Реализация может быть любой: PostgreSQL, MongoDB, in-memory
```

### Single Responsibility (Единственная ответственность)

Каждый модуль делает одну вещь хорошо:

- Handler - только HTTP
- UseCase - только бизнес-логика
- Repository - только данные

### Open/Closed Principle (Открыт для расширения, закрыт для изменения)

Легко добавить новую функциональность без изменения существующего кода:

```go
// Добавить новую сущность "Project" - создать новые файлы
// Старый код не трогаем
domain/project.go
usecase/project/project.go
repository/postgres/project_repo.go
api/handler/project.go
```

## Следующие шаги

- [Подробнее о слоях](./layers.md)
- [Структура директорий](./directory-structure.md)
- [Потоки данных](./data-flow.md)

