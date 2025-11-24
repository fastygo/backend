# CRM для веб-студии

Система управления клиентами и проектами для веб-студии.

## Описание

Веб-студия нуждается в системе для:
- Управления клиентами (компании, контакты)
- Отслеживания проектов (веб-сайты, приложения)
- Управления задачами по проектам
- Отслеживания статусов проектов

## Сущности

### 1. Client (Клиент)

```go
// domain/client.go
type Client struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`         // Название компании
    Email       string            `json:"email"`
    Phone       string            `json:"phone"`
    Website     string            `json:"website,omitempty"`
    Industry    string            `json:"industry,omitempty"`  // Отрасль
    Status      string            `json:"status"`       // active, inactive, archived
    Notes       string            `json:"notes,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 2. Project (Проект)

```go
// domain/project.go
type Project struct {
    ID          string            `json:"id"`
    ClientID    string            `json:"client_id"`
    Name        string            `json:"name"`         // Название проекта
    Description string            `json:"description"`
    Type        string            `json:"type"`        // website, app, landing
    Status      string            `json:"status"`      // planning, development, testing, completed
    Budget      float64           `json:"budget,omitempty"`
    StartDate   *time.Time        `json:"start_date,omitempty"`
    EndDate     *time.Time        `json:"end_date,omitempty"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 3. Contact (Контактное лицо)

```go
// domain/contact.go
type Contact struct {
    ID          string    `json:"id"`
    ClientID    string    `json:"client_id"`
    FirstName   string    `json:"first_name"`
    LastName    string    `json:"last_name"`
    Email       string    `json:"email"`
    Phone       string    `json:"phone,omitempty"`
    Position    string    `json:"position,omitempty"`  // Должность
    IsPrimary   bool      `json:"is_primary"`          // Основной контакт
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

## Реализация

### Шаг 1: Domain

Создайте файлы в `domain/`:

```bash
domain/
├── client.go
├── project.go
└── contact.go
```

### Шаг 2: Repository Interfaces

```go
// repository/client.go
type ClientRepository interface {
    GetByID(ctx context.Context, id string) (*domain.Client, error)
    List(ctx context.Context, filter ClientFilter) ([]domain.Client, error)
    Create(ctx context.Context, client *domain.Client) (*domain.Client, error)
    Update(ctx context.Context, client *domain.Client) error
    Delete(ctx context.Context, id string) error
}

type ClientFilter struct {
    Status string
    Search string  // Поиск по названию или email
    Limit  int
    Offset int
}
```

### Шаг 3: Repository Implementation

```go
// repository/postgres/client_repo.go
type clientRepository struct {
    pool *pgxpool.Pool
}

func NewClientRepository(pool *pgxpool.Pool) repository.ClientRepository {
    return &clientRepository{pool: pool}
}

func (r *clientRepository) Create(ctx context.Context, client *domain.Client) (*domain.Client, error) {
    if client.ID == "" {
        client.ID = uuid.NewString()
    }
    
    query := `
        INSERT INTO clients (id, name, email, phone, website, industry, status, notes, metadata)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        RETURNING created_at, updated_at
    `
    
    metadata := marshalMap(client.Metadata)
    
    err := r.pool.QueryRow(ctx, query,
        client.ID, client.Name, client.Email, client.Phone,
        client.Website, client.Industry, client.Status,
        client.Notes, metadata,
    ).Scan(&client.CreatedAt, &client.UpdatedAt)
    
    if err != nil {
        return nil, err
    }
    
    return client, nil
}
```

### Шаг 4: Use Case

```go
// usecase/client/client.go
type UseCase struct {
    repo   repository.ClientRepository
    logger *zap.Logger
}

func NewUseCase(repo repository.ClientRepository, logger *zap.Logger) *UseCase {
    return &UseCase{repo: repo, logger: logger}
}

func (uc *UseCase) CreateClient(ctx context.Context, client *domain.Client) (*domain.Client, error) {
    // Валидация
    if client.Name == "" {
        return nil, domain.ErrInvalidPayload
    }
    
    // Бизнес-правила
    if client.Status == "" {
        client.Status = "active"
    }
    
    // Сохранение
    return uc.repo.Create(ctx, client)
}
```

### Шаг 5: Handler

```go
// api/handler/client.go
type ClientHandler struct {
    baseHandler
    uc *clientUC.UseCase
}

func NewClientHandler(uc *clientUC.UseCase, adapter *httpcontext.Adapter, logger *zap.Logger) *ClientHandler {
    return &ClientHandler{
        baseHandler: newBaseHandler(adapter, logger),
        uc:          uc,
    }
}

func (h *ClientHandler) CreateClient(ctx *fasthttp.RequestCtx) {
    var req transport.ClientRequest
    if err := json.Unmarshal(ctx.PostBody(), &req); err != nil {
        h.respondJSON(ctx, http.StatusBadRequest, transport.NewError(...))
        return
    }
    
    client := &domain.Client{
        Name:     req.Name,
        Email:    req.Email,
        Phone:    req.Phone,
        Website:  req.Website,
        Industry: req.Industry,
        Notes:    req.Notes,
    }
    
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()
    
    created, err := h.uc.CreateClient(stdCtx, client)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    h.respondSuccess(ctx, http.StatusCreated, created)
}
```

### Шаг 6: Router

```go
// internal/router/router.go
r.POST("/api/v1/clients", authMiddleware(handlers.Client.CreateClient))
r.GET("/api/v1/clients", authMiddleware(handlers.Client.GetClients))
r.GET("/api/v1/clients/{id}", authMiddleware(handlers.Client.GetClient))
r.PUT("/api/v1/clients/{id}", authMiddleware(handlers.Client.UpdateClient))
r.DELETE("/api/v1/clients/{id}", authMiddleware(handlers.Client.DeleteClient))
```

## Миграции БД

```sql
-- assets/migrations/004_crm_clients.sql
CREATE TABLE clients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    phone VARCHAR(50),
    website VARCHAR(255),
    industry VARCHAR(100),
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    notes TEXT,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_clients_status ON clients(status);
CREATE INDEX idx_clients_email ON clients(email);
CREATE INDEX idx_clients_name ON clients(name);

-- assets/migrations/005_crm_projects.sql
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    client_id UUID NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50),
    status VARCHAR(50) NOT NULL DEFAULT 'planning',
    budget DECIMAL(10, 2),
    start_date TIMESTAMP WITH TIME ZONE,
    end_date TIMESTAMP WITH TIME ZONE,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_projects_client_id ON projects(client_id);
CREATE INDEX idx_projects_status ON projects(status);
```

## API Endpoints

### Clients

- `POST /api/v1/clients` - Создать клиента
- `GET /api/v1/clients` - Список клиентов
- `GET /api/v1/clients/{id}` - Получить клиента
- `PUT /api/v1/clients/{id}` - Обновить клиента
- `DELETE /api/v1/clients/{id}` - Удалить клиента

### Projects

- `POST /api/v1/projects` - Создать проект
- `GET /api/v1/projects` - Список проектов
- `GET /api/v1/projects/{id}` - Получить проект
- `PUT /api/v1/projects/{id}` - Обновить проект
- `GET /api/v1/clients/{id}/projects` - Проекты клиента

## Примеры использования

### Создание клиента

```bash
curl -X POST http://localhost:8080/api/v1/clients \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "ООО ВебСтудия",
    "email": "info@webstudio.ru",
    "phone": "+7 (999) 123-45-67",
    "website": "https://webstudio.ru",
    "industry": "IT"
  }'
```

### Создание проекта

```bash
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Корпоративный сайт",
    "description": "Разработка корпоративного сайта",
    "type": "website",
    "status": "planning",
    "budget": 500000
  }'
```

## Расширение функциональности

### Добавить задачи к проектам

Используйте существующую сущность `Task` и свяжите с проектами:

```sql
ALTER TABLE tasks ADD COLUMN project_id UUID REFERENCES projects(id);
```

### Добавить контакты клиентов

Реализуйте сущность `Contact` аналогично `Client`.

### Добавить временные треки

Создайте сущность `TimeEntry` для учета времени работы над проектами.

## Следующие шаги

- [Кофейня CRM](./coffee-shop.md) - Другой пример CRM
- [Общая документация CRM](../crm/README.md)

