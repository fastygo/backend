# Мебельный магазин

Интернет-магазин мебели с каталогом товаров, корзиной и заказами.

## Описание

Интернет-магазин мебели нуждается в системе для:
- Управления каталогом товаров
- Управления категориями мебели
- Корзины покупок
- Оформления заказов
- Управления складом

## Сущности

### 1. Product (Товар)

```go
// domain/product.go
type Product struct {
    ID          string            `json:"id"`
    Name        string            `json:"name"`
    Description string            `json:"description"`
    SKU         string            `json:"sku"`         // Артикул
    CategoryID  string            `json:"category_id"`
    Price       float64           `json:"price"`
    Stock       int               `json:"stock"`       // Количество на складе
    Images      []string          `json:"images"`      // URL изображений
    Attributes  map[string]string `json:"attributes"` // Размеры, цвет, материал
    IsActive    bool              `json:"is_active"`
    CreatedAt   time.Time         `json:"created_at"`
    UpdatedAt   time.Time         `json:"updated_at"`
}
```

### 2. Category (Категория)

```go
// domain/category.go
type Category struct {
    ID          string    `json:"id"`
    Name        string    `json:"name"`
    Slug        string    `json:"slug"`
    ParentID    *string   `json:"parent_id,omitempty"`  // Для вложенных категорий
    Description string    `json:"description,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### 3. Cart (Корзина)

```go
// domain/cart.go
type Cart struct {
    ID        string      `json:"id"`
    UserID    string      `json:"user_id"`
    Items     []CartItem  `json:"items"`
    Total     float64     `json:"total"`
    UpdatedAt time.Time   `json:"updated_at"`
}

type CartItem struct {
    ProductID string  `json:"product_id"`
    Quantity  int     `json:"quantity"`
    Price     float64 `json:"price"`
}
```

### 4. Order (Заказ)

```go
// domain/order.go
type Order struct {
    ID          string         `json:"id"`
    UserID      string         `json:"user_id"`
    Items       []OrderItem    `json:"items"`
    Total       float64        `json:"total"`
    Status      string         `json:"status"`  // pending, paid, shipped, delivered, cancelled
    ShippingAddress Address     `json:"shipping_address"`
    PaymentMethod   string      `json:"payment_method"`
    CreatedAt       time.Time   `json:"created_at"`
    UpdatedAt       time.Time   `json:"updated_at"`
}

type Address struct {
    Street  string `json:"street"`
    City    string `json:"city"`
    ZipCode string `json:"zip_code"`
    Country string `json:"country"`
}
```

## Реализация

### Проверка наличия товара

```go
// usecase/product/product.go
func (uc *UseCase) CheckAvailability(ctx context.Context, productID string, quantity int) (bool, error) {
    product, err := uc.repo.GetByID(ctx, productID)
    if err != nil {
        return false, err
    }
    
    if !product.IsActive {
        return false, domain.ErrProductNotAvailable
    }
    
    return product.Stock >= quantity, nil
}
```

### Создание заказа из корзины

```go
// usecase/order/order.go
func (uc *UseCase) CreateFromCart(ctx context.Context, userID string, address Address) (*domain.Order, error) {
    // Получаем корзину
    cart, err := uc.cartRepo.GetByUserID(ctx, userID)
    if err != nil {
        return nil, err
    }
    
    if len(cart.Items) == 0 {
        return nil, domain.ErrCartEmpty
    }
    
    // Проверяем наличие всех товаров
    for _, item := range cart.Items {
        available, err := uc.productUC.CheckAvailability(ctx, item.ProductID, item.Quantity)
        if err != nil {
            return nil, err
        }
        if !available {
            return nil, &domain.DomainError{
                Code:    domain.ErrCodeInvalid,
                Message: "product not available",
            }
        }
    }
    
    // Создаем заказ
    order := &domain.Order{
        UserID:          userID,
        Items:           convertCartItemsToOrderItems(cart.Items),
        Total:           cart.Total,
        Status:          "pending",
        ShippingAddress: address,
    }
    
    created, err := uc.repo.Create(ctx, order)
    if err != nil {
        return nil, err
    }
    
    // Уменьшаем количество товаров на складе
    for _, item := range cart.Items {
        uc.productUC.DecreaseStock(ctx, item.ProductID, item.Quantity)
    }
    
    // Очищаем корзину
    uc.cartRepo.Clear(ctx, userID)
    
    return created, nil
}
```

## Миграции БД

```sql
-- assets/migrations/017_store_products.sql
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    sku VARCHAR(100) UNIQUE NOT NULL,
    category_id UUID NOT NULL REFERENCES categories(id),
    price DECIMAL(10, 2) NOT NULL,
    stock INTEGER NOT NULL DEFAULT 0,
    images TEXT[],
    attributes JSONB,
    is_active BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_products_category_id ON products(category_id);
CREATE INDEX idx_products_sku ON products(sku);
CREATE INDEX idx_products_is_active ON products(is_active);

-- assets/migrations/018_store_carts.sql
CREATE TABLE carts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    items JSONB NOT NULL,
    total DECIMAL(10, 2) NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    UNIQUE(user_id)
);

CREATE INDEX idx_carts_user_id ON carts(user_id);

-- assets/migrations/019_store_orders.sql
CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id),
    items JSONB NOT NULL,
    total DECIMAL(10, 2) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    shipping_address JSONB NOT NULL,
    payment_method VARCHAR(50),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_created_at ON orders(created_at);
```

## API Endpoints

### Products

- `GET /api/v1/products` - Каталог товаров
- `GET /api/v1/products/{id}` - Детали товара
- `POST /api/v1/products` - Создать товар (админ)
- `PUT /api/v1/products/{id}` - Обновить товар (админ)

### Cart

- `GET /api/v1/cart` - Получить корзину
- `POST /api/v1/cart/items` - Добавить товар в корзину
- `PUT /api/v1/cart/items/{product_id}` - Обновить количество
- `DELETE /api/v1/cart/items/{product_id}` - Удалить из корзины
- `DELETE /api/v1/cart` - Очистить корзину

### Orders

- `POST /api/v1/orders` - Создать заказ из корзины
- `GET /api/v1/orders` - Мои заказы
- `GET /api/v1/orders/{id}` - Детали заказа
- `PUT /api/v1/orders/{id}/status` - Обновить статус (админ)

## Примеры использования

### Добавить товар в корзину

```bash
curl -X POST http://localhost:8080/api/v1/cart/items \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "product_id": "prod-123",
    "quantity": 2
  }'
```

### Создать заказ

```bash
curl -X POST http://localhost:8080/api/v1/orders \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "shipping_address": {
      "street": "ул. Ленина, д. 1",
      "city": "Москва",
      "zip_code": "123456",
      "country": "Россия"
    },
    "payment_method": "card"
  }'
```

## Расширение функциональности

### Поиск товаров

Добавьте полнотекстовый поиск по названию и описанию.

### Фильтрация

Фильтры по цене, категории, атрибутам (размер, цвет).

### Отзывы и рейтинги

Добавьте возможность оставлять отзывы о товарах.

## Следующие шаги

- [Блог](./blog.md) - Другой пример CMS
- [Общая документация CMS](./README.md)

