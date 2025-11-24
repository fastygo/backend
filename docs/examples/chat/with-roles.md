# Чат с системой ролей

Чат с поддержкой ролей и агентов поддержки - идеально для customer support систем.

## Описание

Расширенный чат с:
- Системой ролей (admin, agent, member)
- Агентами поддержки
- Приоритетами сообщений
- Тикетами поддержки

## Сущности

Расширяем базовые сущности из [простого чата](./simple.md):

### 1. Room с ролями

```go
// domain/room.go (расширенная версия)
type Room struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Type        string            `json:"type"`  // public, private, support
    CreatedBy   string            `json:"created_by"`
    Members     []RoomMember      `json:"members,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}

type RoomMember struct {
    ID        string    `json:"id"`
    RoomID    string    `json:"room_id"`
    UserID    string    `json:"user_id"`
    Role      string    `json:"role"`  // admin, agent, member
    JoinedAt  time.Time `json:"joined_at"`
}
```

### 2. Message с приоритетом

```go
// domain/message.go (расширенная версия)
type Message struct {
    ID        string    `json:"id"`
    RoomID    string    `json:"room_id"`
    UserID    string    `json:"user_id"`
    Content   string    `json:"content"`
    Priority  string    `json:"priority"`  // low, normal, high, urgent
    Status    string    `json:"status"`    // sent, delivered, read
    CreatedAt time.Time `json:"created_at"`
}
```

### 3. Ticket (Тикет поддержки)

```go
// domain/ticket.go
type Ticket struct {
    ID          string     `json:"id"`
    RoomID      string     `json:"room_id"`
    CustomerID  string     `json:"customer_id"`
    AgentID     *string    `json:"agent_id,omitempty"`
    Subject     string     `json:"subject"`
    Status      string     `json:"status"`  // open, assigned, resolved, closed
    Priority    string     `json:"priority"`
    CreatedAt   time.Time  `json:"created_at"`
    ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
}
```

## Реализация

### Проверка ролей в Use Case

```go
// usecase/room/room.go
func (uc *UseCase) AddMember(ctx context.Context, roomID, userID, role string) error {
    // Проверяем права
    callerID := getUserIDFromContext(ctx)
    callerRole, err := uc.getMemberRole(ctx, roomID, callerID)
    if err != nil {
        return err
    }
    
    // Только admin может добавлять участников
    if callerRole != "admin" {
        return domain.ErrForbidden
    }
    
    // Добавляем участника
    member := &domain.RoomMember{
        RoomID: roomID,
        UserID: userID,
        Role:   role,
    }
    
    return uc.repo.AddMember(ctx, member)
}
```

### Назначение агента на тикет

```go
// usecase/ticket/ticket.go
func (uc *UseCase) AssignAgent(ctx context.Context, ticketID, agentID string) error {
    ticket, err := uc.repo.GetByID(ctx, ticketID)
    if err != nil {
        return err
    }
    
    // Проверяем, что пользователь - агент
    agent, err := uc.userRepo.GetByID(ctx, agentID)
    if err != nil {
        return err
    }
    
    if agent.Role != "agent" && agent.Role != "admin" {
        return domain.ErrForbidden
    }
    
    ticket.AgentID = &agentID
    ticket.Status = "assigned"
    
    return uc.repo.Update(ctx, ticket)
}
```

## Миграции БД

```sql
-- assets/migrations/015_chat_roles.sql
ALTER TABLE room_members ADD COLUMN role VARCHAR(50) NOT NULL DEFAULT 'member';

CREATE INDEX idx_room_members_role ON room_members(role);

-- assets/migrations/016_chat_tickets.sql
CREATE TABLE tickets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES users(id),
    agent_id UUID REFERENCES users(id),
    subject VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'open',
    priority VARCHAR(50) NOT NULL DEFAULT 'normal',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    resolved_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_tickets_status ON tickets(status);
CREATE INDEX idx_tickets_agent_id ON tickets(agent_id);
CREATE INDEX idx_tickets_customer_id ON tickets(customer_id);
```

## API Endpoints

### Rooms с ролями

- `POST /api/v1/rooms/{id}/members` - Добавить участника (требует admin)
- `PUT /api/v1/rooms/{id}/members/{user_id}/role` - Изменить роль (требует admin)
- `DELETE /api/v1/rooms/{id}/members/{user_id}` - Удалить участника (требует admin)

### Tickets

- `POST /api/v1/tickets` - Создать тикет
- `GET /api/v1/tickets` - Список тикетов
- `PUT /api/v1/tickets/{id}/assign` - Назначить агента
- `PUT /api/v1/tickets/{id}/resolve` - Решить тикет

## Примеры использования

### Создать тикет поддержки

```bash
curl -X POST http://localhost:8080/api/v1/tickets \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "subject": "Проблема с заказом",
    "priority": "high",
    "room_id": "room-123"
  }'
```

### Назначить агента на тикет

```bash
curl -X PUT http://localhost:8080/api/v1/tickets/{ticket_id}/assign \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agent-456"
  }'
```

## Следующие шаги

- [Простой чат](./simple.md) - Базовая версия
- [Общая документация чатов](./README.md)

