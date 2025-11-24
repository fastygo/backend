# Дашборд для учета проектов и задач

Система для управления проектами и задачами - идеально подходит для команд разработки.

## Описание

Дашборд помогает:
- Управлять проектами
- Отслеживать задачи по проектам
- Видеть статистику и аналитику
- Управлять командой

## Сущности

Проект уже содержит базовые сущности `Task` и `User`. Для дашборда нужно добавить:

### 1. Project (Проект)

```go
// domain/project.go
type Project struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    OwnerID     string            `json:"owner_id"`
    Status      string            `json:"status"`  // active, completed, archived
    StartDate   *time.Time        `json:"start_date,omitempty"`
    EndDate     *time.Time        `json:"end_date,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 2. ProjectMember (Участник проекта)

```go
// domain/project_member.go
type ProjectMember struct {
    ID        string    `json:"id"`
    ProjectID string    `json:"project_id"`
    UserID    string    `json:"user_id"`
    Role      string    `json:"role"`  // owner, admin, member, viewer
    CreatedAt time.Time `json:"created_at"`
}
```

## Статистика

### Endpoint для статистики

```go
// api/handler/dashboard.go
func (h *DashboardHandler) GetStats(ctx *fasthttp.RequestCtx) {
    userID := h.userID(ctx)
    
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()
    
    stats, err := h.uc.GetStats(stdCtx, userID)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    h.respondSuccess(ctx, http.StatusOK, stats)
}
```

### Use Case для статистики

```go
// usecase/dashboard/dashboard.go
type Stats struct {
    TotalProjects    int `json:"total_projects"`
    ActiveProjects   int `json:"active_projects"`
    TotalTasks       int `json:"total_tasks"`
    CompletedTasks   int `json:"completed_tasks"`
    PendingTasks     int `json:"pending_tasks"`
    OverdueTasks     int `json:"overdue_tasks"`
}

func (uc *UseCase) GetStats(ctx context.Context, userID string) (*Stats, error) {
    // Получаем проекты пользователя
    projects, err := uc.projectRepo.ListByUser(ctx, userID)
    if err != nil {
        return nil, err
    }
    
    // Получаем задачи пользователя
    tasks, err := uc.taskRepo.List(ctx, repository.TaskFilter{UserID: userID})
    if err != nil {
        return nil, err
    }
    
    stats := &Stats{
        TotalProjects: len(projects),
        TotalTasks:    len(tasks),
    }
    
    // Подсчет статистики
    for _, project := range projects {
        if project.Status == "active" {
            stats.ActiveProjects++
        }
    }
    
    now := time.Now()
    for _, task := range tasks {
        switch task.Status {
        case "completed":
            stats.CompletedTasks++
        case "pending":
            stats.PendingTasks++
            if task.DueDate != nil && task.DueDate.Before(now) {
                stats.OverdueTasks++
            }
        }
    }
    
    return stats, nil
}
```

## API Endpoints

### Dashboard

- `GET /api/v1/dashboard/stats` - Статистика пользователя
- `GET /api/v1/dashboard/projects` - Проекты пользователя
- `GET /api/v1/dashboard/tasks` - Задачи пользователя

### Projects

- `POST /api/v1/projects` - Создать проект
- `GET /api/v1/projects` - Список проектов
- `GET /api/v1/projects/{id}` - Получить проект
- `PUT /api/v1/projects/{id}` - Обновить проект
- `DELETE /api/v1/projects/{id}` - Удалить проект

## Примеры использования

### Получить статистику

```bash
curl http://localhost:8080/api/v1/dashboard/stats \
  -H "Authorization: Bearer <token>"
```

Ответ:
```json
{
  "success": true,
  "data": {
    "total_projects": 5,
    "active_projects": 3,
    "total_tasks": 42,
    "completed_tasks": 28,
    "pending_tasks": 12,
    "overdue_tasks": 2
  }
}
```

### Получить проекты с задачами

```bash
curl http://localhost:8080/api/v1/dashboard/projects?include_tasks=true \
  -H "Authorization: Bearer <token>"
```

## Следующие шаги

- Изучите существующие сущности `Task` и `User`
- Добавьте `Project` по аналогии
- Реализуйте статистику
- Создайте фронтенд для визуализации

