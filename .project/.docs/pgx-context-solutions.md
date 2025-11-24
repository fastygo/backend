# Решения для совместимости pgx context с fasthttp

## Проблема

Библиотека `fasthttp` использует свой контекст `*fasthttp.RequestCtx`, который не совместим напрямую со стандартным `context.Context`, требуемым для `pgx` (PostgreSQL драйвер).

## Варианты решения

### Вариант 1: Создание стандартного context.Context с таймаутом (Рекомендуемый)

**Преимущества:**
- Простой и понятный подход
- Автоматическая отмена при таймауте
- Соответствует best practices Go

**Недостатки:**
- Теряется связь с оригинальным RequestCtx (но это обычно не нужно)

**Реализация:**

```go
// api/handler/profile.go
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
    userID := string(ctx.Request.Header.Peek("X-User-ID"))
    if userID == "" {
        h.respondError(ctx, fasthttp.StatusUnauthorized, "missing user ID")
        return
    }

    // Create standard context with timeout
    reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    user, err := h.profileUseCase.GetProfile(reqCtx, userID)
    if err != nil {
        h.logger.Error("failed to get profile", zap.Error(err))
        h.respondError(ctx, fasthttp.StatusInternalServerError, "internal error")
        return
    }

    h.respondSuccess(ctx, user)
}
```

### Вариант 2: Context с передачей данных через context.WithValue

**Преимущества:**
- Сохраняет доступ к данным запроса (userID, requestID и т.д.)
- Позволяет логировать с request ID
- Удобно для трейсинга

**Недостатки:**
- Нужно аккуратно использовать context.WithValue (только для request-scoped данных)

**Реализация:**

```go
// internal/context/request.go
package contextutil

import (
    "context"
    "time"
    "github.com/valyala/fasthttp"
)

type requestKey struct{}

type RequestData struct {
    UserID    string
    RequestID string
    IP        string
}

func WithRequestData(ctx *fasthttp.RequestCtx) (context.Context, context.CancelFunc) {
    reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    
    data := RequestData{
        UserID:    string(ctx.Request.Header.Peek("X-User-ID")),
        RequestID: string(ctx.Request.Header.Peek("X-Request-ID")),
        IP:        ctx.RemoteIP().String(),
    }
    
    return context.WithValue(reqCtx, requestKey{}, data), cancel
}

func GetRequestData(ctx context.Context) *RequestData {
    if data, ok := ctx.Value(requestKey{}).(RequestData); ok {
        return &data
    }
    return nil
}

// Usage in handler:
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
    reqCtx, cancel := contextutil.WithRequestData(ctx)
    defer cancel()

    data := contextutil.GetRequestData(reqCtx)
    if data == nil || data.UserID == "" {
        h.respondError(ctx, fasthttp.StatusUnauthorized, "missing user ID")
        return
    }

    user, err := h.profileUseCase.GetProfile(reqCtx, data.UserID)
    // ...
}
```

### Вариант 3: Использование context.Background() с ручной отменой

**Преимущества:**
- Максимально простой подход
- Нет зависимости от RequestCtx

**Недостатки:**
- Нет автоматической отмены при завершении запроса
- Может привести к утечкам ресурсов при долгих запросах

**Реализация:**

```go
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
    userID := string(ctx.Request.Header.Peek("X-User-ID"))
    if userID == "" {
        h.respondError(ctx, fasthttp.StatusUnauthorized, "missing user ID")
        return
    }

    // Use background context - NOT RECOMMENDED for production
    user, err := h.profileUseCase.GetProfile(context.Background(), userID)
    // ...
}
```

**⚠️ Не рекомендуется для production** - нет контроля над таймаутами.

### Вариант 4: Адаптер с поддержкой отмены через RequestCtx.Done()

**Преимущества:**
- Связь с жизненным циклом fasthttp запроса
- Автоматическая отмена при закрытии соединения
- Более точный контроль

**Недостатки:**
- Более сложная реализация
- Требует дополнительной логики

**Реализация:**

```go
// internal/context/fasthttp_adapter.go
package contextutil

import (
    "context"
    "time"
    "github.com/valyala/fasthttp"
)

// FastHTTPContext creates a context.Context from fasthttp.RequestCtx
func FastHTTPContext(ctx *fasthttp.RequestCtx) (context.Context, context.CancelFunc) {
    // Create context with timeout
    reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    
    // Monitor RequestCtx for connection close
    go func() {
        select {
        case <-ctx.Done():
            cancel() // Cancel if fasthttp context is done
        case <-reqCtx.Done():
            // Context already cancelled
        }
    }()
    
    return reqCtx, cancel
}

// Usage:
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
    reqCtx, cancel := contextutil.FastHTTPContext(ctx)
    defer cancel()

    userID := string(ctx.Request.Header.Peek("X-User-ID"))
    // ...
}
```

### Вариант 5: Middleware для создания контекста

**Преимущества:**
- Централизованная логика создания контекста
- Единообразный подход во всех handlers
- Легко добавить общие значения (request ID, user ID)

**Недостатки:**
- Требует изменения архитектуры handlers

**Реализация:**

```go
// internal/middleware/context.go
package middleware

import (
    "context"
    "time"
    "github.com/valyala/fasthttp"
    "github.com/google/uuid"
)

type ContextKey string

const (
    KeyUserID    ContextKey = "user_id"
    KeyRequestID ContextKey = "request_id"
)

func ContextMiddleware(next fasthttp.RequestHandler) fasthttp.RequestHandler {
    return func(ctx *fasthttp.RequestCtx) {
        // Generate request ID if not present
        requestID := string(ctx.Request.Header.Peek("X-Request-ID"))
        if requestID == "" {
            requestID = uuid.New().String()
            ctx.Request.Header.Set("X-Request-ID", requestID)
        }

        // Create standard context with timeout
        reqCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer cancel()

        // Add request-scoped values
        reqCtx = context.WithValue(reqCtx, KeyRequestID, requestID)
        if userID := string(ctx.Request.Header.Peek("X-User-ID")); userID != "" {
            reqCtx = context.WithValue(reqCtx, KeyUserID, userID)
        }

        // Store context in RequestCtx user values
        ctx.SetUserValue("std_ctx", reqCtx)

        next(ctx)
    }
}

// Helper to extract context from RequestCtx
func GetContext(ctx *fasthttp.RequestCtx) context.Context {
    if stdCtx, ok := ctx.UserValue("std_ctx").(context.Context); ok {
        return stdCtx
    }
    // Fallback to background context with timeout
    reqCtx, _ := context.WithTimeout(context.Background(), 30*time.Second)
    return reqCtx
}

// Usage in handler:
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
    reqCtx := middleware.GetContext(ctx)
    
    userID, ok := reqCtx.Value(middleware.KeyUserID).(string)
    if !ok || userID == "" {
        h.respondError(ctx, fasthttp.StatusUnauthorized, "missing user ID")
        return
    }

    user, err := h.profileUseCase.GetProfile(reqCtx, userID)
    // ...
}
```

### Вариант 6: Использование context.WithCancel и мониторинг RequestCtx

**Преимущества:**
- Полный контроль над жизненным циклом
- Можно добавить кастомную логику отмены

**Недостатки:**
- Более сложная реализация
- Требует дополнительных горутин

**Реализация:**

```go
// internal/context/fasthttp_context.go
package contextutil

import (
    "context"
    "time"
    "github.com/valyala/fasthttp"
)

func NewContextFromFastHTTP(ctx *fasthttp.RequestCtx, timeout time.Duration) (context.Context, context.CancelFunc) {
    reqCtx, cancel := context.WithCancel(context.Background())
    
    // Set timeout
    if timeout > 0 {
        reqCtx, cancel = context.WithTimeout(reqCtx, timeout)
    }

    // Monitor fasthttp context
    go func() {
        <-ctx.Done()
        cancel()
    }()

    return reqCtx, cancel
}
```

## Рекомендации

### Для простых случаев (Рекомендуется)
Используйте **Вариант 1** - создание context с таймаутом. Это самый простой и надежный подход.

### Для production с трейсингом
Используйте **Вариант 2** или **Вариант 5** - они позволяют передавать request-scoped данные (user ID, request ID) через context, что полезно для логирования и мониторинга.

### Для максимального контроля
Используйте **Вариант 4** или **Вариант 6** - они обеспечивают связь с жизненным циклом fasthttp запроса.

## Пример полной реализации (Вариант 2)

```go
// internal/context/request.go
package contextutil

import (
    "context"
    "time"
    "github.com/valyala/fasthttp"
)

const defaultTimeout = 30 * time.Second

type key string

const (
    userIDKey    key = "user_id"
    requestIDKey key = "request_id"
    ipKey        key = "ip"
)

// NewRequestContext creates a standard context.Context from fasthttp.RequestCtx
func NewRequestContext(ctx *fasthttp.RequestCtx) (context.Context, context.CancelFunc) {
    return NewRequestContextWithTimeout(ctx, defaultTimeout)
}

// NewRequestContextWithTimeout creates context with custom timeout
func NewRequestContextWithTimeout(ctx *fasthttp.RequestCtx, timeout time.Duration) (context.Context, context.CancelFunc) {
    reqCtx, cancel := context.WithTimeout(context.Background(), timeout)
    
    // Extract and store request-scoped values
    if userID := string(ctx.Request.Header.Peek("X-User-ID")); userID != "" {
        reqCtx = context.WithValue(reqCtx, userIDKey, userID)
    }
    
    if requestID := string(ctx.Request.Header.Peek("X-Request-ID")); requestID != "" {
        reqCtx = context.WithValue(reqCtx, requestIDKey, requestID)
    }
    
    reqCtx = context.WithValue(reqCtx, ipKey, ctx.RemoteIP().String())
    
    return reqCtx, cancel
}

// GetUserID extracts user ID from context
func GetUserID(ctx context.Context) string {
    if userID, ok := ctx.Value(userIDKey).(string); ok {
        return userID
    }
    return ""
}

// GetRequestID extracts request ID from context
func GetRequestID(ctx context.Context) string {
    if requestID, ok := ctx.Value(requestIDKey).(string); ok {
        return requestID
    }
    return ""
}

// GetIP extracts IP address from context
func GetIP(ctx context.Context) string {
    if ip, ok := ctx.Value(ipKey).(string); ok {
        return ip
    }
    return ""
}
```

**Использование в handler:**

```go
// api/handler/profile.go
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
    reqCtx, cancel := contextutil.NewRequestContext(ctx)
    defer cancel()

    userID := contextutil.GetUserID(reqCtx)
    if userID == "" {
        h.respondError(ctx, fasthttp.StatusUnauthorized, "missing user ID")
        return
    }

    user, err := h.profileUseCase.GetProfile(reqCtx, userID)
    if err != nil {
        h.logger.Error("failed to get profile",
            zap.Error(err),
            zap.String("request_id", contextutil.GetRequestID(reqCtx)),
            zap.String("user_id", userID),
        )
        h.respondError(ctx, fasthttp.StatusInternalServerError, "internal error")
        return
    }

    h.respondSuccess(ctx, user)
}
```

## Важные замечания

1. **Всегда используйте defer cancel()** - это предотвращает утечки ресурсов
2. **Устанавливайте разумные таймауты** - обычно 30 секунд достаточно для большинства запросов
3. **Не храните RequestCtx в context.Value** - это может привести к утечкам памяти
4. **Используйте context.WithValue только для request-scoped данных** - не для бизнес-логики
5. **Мониторьте отмену контекста** - pgx автоматически отменит запросы при отмене контекста

## Альтернативные подходы

### Использование других HTTP фреймворков

Если совместимость контекстов критична, рассмотрите альтернативы:

- **Gin** - использует стандартный `context.Context`
- **Echo** - использует стандартный `context.Context`
- **Chi** - использует стандартный `context.Context`
- **net/http** - стандартная библиотека с `context.Context`

Однако fasthttp обеспечивает лучшую производительность, поэтому обычно стоит использовать один из предложенных вариантов адаптации.

