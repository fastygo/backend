# Простой чат

Базовый чат без системы ролей - идеально для простых приложений обмена сообщениями.

## Описание

Простой чат позволяет:
- Создавать комнаты для общения
- Отправлять сообщения в комнаты
- Просматривать историю сообщений
- Видеть список участников

## Сущности

### 1. Room (Комната)

```go
// domain/room.go
type Room struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description,omitempty"`
    Type        string            `json:"type"`  // public, private
    CreatedBy   string            `json:"created_by"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 2. Message (Сообщение)

```go
// domain/message.go
type Message struct {
    ID        string    `json:"id"`
    RoomID    string    `json:"room_id"`
    UserID    string    `json:"user_id"`
    Content   string    `json:"content"`
    Type      string    `json:"type"`  // text, image, file
    CreatedAt time.Time `json:"created_at"`
}
```

### 3. RoomMember (Участник комнаты)

```go
// domain/room_member.go
type RoomMember struct {
    ID        string    `json:"id"`
    RoomID    string    `json:"room_id"`
    UserID    string    `json:"user_id"`
    JoinedAt  time.Time `json:"joined_at"`
}
```

## Реализация

Следуйте стандартной структуре:
1. Domain → Repository → Use Case → Handler → Router

## Миграции БД

```sql
-- assets/migrations/012_chat_rooms.sql
CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL DEFAULT 'public',
    created_by UUID NOT NULL REFERENCES users(id),
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_rooms_type ON rooms(type);
CREATE INDEX idx_rooms_created_by ON rooms(created_by);

-- assets/migrations/013_chat_messages.sql
CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    content TEXT NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'text',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_messages_room_id ON messages(room_id);
CREATE INDEX idx_messages_created_at ON messages(created_at);

-- assets/migrations/014_chat_room_members.sql
CREATE TABLE room_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id),
    joined_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(room_id, user_id)
);

CREATE INDEX idx_room_members_room_id ON room_members(room_id);
CREATE INDEX idx_room_members_user_id ON room_members(user_id);
```

## API Endpoints

### Rooms

- `POST /api/v1/rooms` - Создать комнату
- `GET /api/v1/rooms` - Список комнат
- `GET /api/v1/rooms/{id}` - Получить комнату
- `POST /api/v1/rooms/{id}/join` - Присоединиться к комнате
- `DELETE /api/v1/rooms/{id}/leave` - Покинуть комнату

### Messages

- `POST /api/v1/rooms/{id}/messages` - Отправить сообщение
- `GET /api/v1/rooms/{id}/messages` - История сообщений
- `DELETE /api/v1/messages/{id}` - Удалить сообщение

## Примеры использования

### Создать комнату

```bash
curl -X POST http://localhost:8080/api/v1/rooms \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Общая комната",
    "description": "Комната для всех",
    "type": "public"
  }'
```

### Отправить сообщение

```bash
curl -X POST http://localhost:8080/api/v1/rooms/{room_id}/messages \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "content": "Привет всем!"
  }'
```

### Получить историю сообщений

```bash
curl http://localhost:8080/api/v1/rooms/{room_id}/messages?limit=50 \
  -H "Authorization: Bearer <token>"
```

## Расширение функциональности

### Real-time обновления

Для real-time обновлений используйте WebSocket:
- Добавьте WebSocket handler
- Отправляйте новые сообщения всем подключенным клиентам

### Уведомления

- Push уведомления о новых сообщениях
- Email уведомления для важных комнат

## Следующие шаги

- [Чат с ролями](./with-roles.md) - Расширенная версия с ролями
- [Общая документация чатов](./README.md)

