# CRM для кофейни

Система управления заказами и клиентами для кофейни.

## Описание

Кофейня нуждается в системе для:
- Управления клиентами (постоянные клиенты, программы лояльности)
- Отслеживания заказов
- Управления меню
- Аналитики продаж

## Сущности

### 1. Customer (Клиент)

```go
// domain/customer.go
type Customer struct {
    ID            string            `json:"id"`
    Name          string            `json:"name"`
    Phone         string            `json:"phone"`
    Email         string            `json:"email,omitempty"`
    LoyaltyPoints int              `json:"loyalty_points"`  // Баллы лояльности
    Status        string            `json:"status"`         // active, blocked
    Metadata      map[string]string `json:"metadata,omitempty"`
    CreatedAt     time.Time         `json:"created_at"`
    UpdatedAt     time.Time         `json:"updated_at"`
}
```

### 2. Order (Заказ)

```go
// domain/order.go
type Order struct {
    ID          string         `json:"id"`
    CustomerID  *string        `json:"customer_id,omitempty"`  // Может быть гостем
    Items       []OrderItem     `json:"items"`
    Total       float64        `json:"total"`
    Status      string         `json:"status"`  // pending, preparing, ready, completed, cancelled
    PaymentType string         `json:"payment_type"`  // cash, card, online
    CreatedAt   time.Time      `json:"created_at"`
    UpdatedAt   time.Time      `json:"updated_at"`
}

type OrderItem struct {
    ProductID string  `json:"product_id"`
    Name      string  `json:"name"`
    Quantity  int     `json:"quantity"`
    Price     float64 `json:"price"`
    Total     float64 `json:"total"`
}
```

### 3. Product (Товар/Напиток)

```go
// domain/product.go
type Product struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description,omitempty"`
    Category    string            `json:"category"`  // coffee, tea, dessert, food
    Price       float64           `json:"price"`
    IsAvailable bool              `json:"is_available"`
    Metadata    map[string]string `json:"metadata,omitempty"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

## Реализация

Следуйте той же структуре, что и в [веб-студии](./webstudio.md):

1. Domain → Repository → Use Case → Handler → Router

## Особенности

### Программа лояльности

```go
// usecase/customer/customer.go
func (uc *UseCase) AddLoyaltyPoints(ctx context.Context, customerID string, points int) error {
    customer, err := uc.repo.GetByID(ctx, customerID)
    if err != nil {
        return err
    }
    
    customer.LoyaltyPoints += points
    
    // Бонус: каждые 100 баллов = бесплатный напиток
    if customer.LoyaltyPoints >= 100 {
        customer.LoyaltyPoints -= 100
        // Уведомление о бесплатном напитке
    }
    
    return uc.repo.Update(ctx, customer)
}
```

### Статистика продаж

```go
// usecase/order/order.go
func (uc *UseCase) GetSalesStats(ctx context.Context, from, to time.Time) (*SalesStats, error) {
    orders, err := uc.repo.ListByDateRange(ctx, from, to)
    if err != nil {
        return nil, err
    }
    
    stats := &SalesStats{
        TotalOrders: len(orders),
        TotalRevenue: 0,
        AverageOrder: 0,
    }
    
    for _, order := range orders {
        stats.TotalRevenue += order.Total
    }
    
    if len(orders) > 0 {
        stats.AverageOrder = stats.TotalRevenue / float64(len(orders))
    }
    
    return stats, nil
}
```

## Миграции БД

```sql
-- assets/migrations/006_coffee_customers.sql
CREATE TABLE customers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    phone VARCHAR(50) UNIQUE,
    email VARCHAR(255),
    loyalty_points INTEGER DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_customers_phone ON customers(phone);

-- assets/migrations/007_coffee_products.sql
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    category VARCHAR(50) NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    is_available BOOLEAN DEFAULT true,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_products_category ON products(category);
CREATE INDEX idx_products_available ON products(is_available);

-- assets/migrations/008_coffee_orders.sql
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    customer_id UUID REFERENCES customers(id),
    items JSONB NOT NULL,
    total DECIMAL(10, 2) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    payment_type VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_orders_customer_id ON orders(customer_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at);
```

## API Endpoints

### Customers

- `POST /api/v1/customers` - Создать клиента
- `GET /api/v1/customers` - Список клиентов
- `GET /api/v1/customers/{id}` - Получить клиента
- `PUT /api/v1/customers/{id}/loyalty` - Добавить баллы лояльности

### Orders

- `POST /api/v1/orders` - Создать заказ
- `GET /api/v1/orders` - Список заказов
- `PUT /api/v1/orders/{id}/status` - Обновить статус заказа
- `GET /api/v1/orders/stats` - Статистика продаж

### Products

- `POST /api/v1/products` - Создать товар
- `GET /api/v1/products` - Список товаров
- `PUT /api/v1/products/{id}` - Обновить товар

## Примеры использования

### Создание заказа

```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "customer_id": "550e8400-e29b-41d4-a716-446655440000",
    "items": [
      {
        "product_id": "prod-123",
        "name": "Капучино",
        "quantity": 2,
        "price": 250,
        "total": 500
      }
    ],
    "total": 500,
    "payment_type": "card"
  }'
```

### Добавление баллов лояльности

```bash
curl -X PUT http://localhost:8080/api/v1/customers/{id}/loyalty \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "points": 10
  }'
```

## Следующие шаги

- [Веб-студия CRM](./webstudio.md) - Другой пример CRM
- [Общая документация CRM](./README.md)

