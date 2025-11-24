# Простой блог

Система управления контентом для блога, похожая на WordPress.

## Описание

Блог нуждается в системе для:
- Управления статьями (постами)
- Управления категориями и тегами
- Комментариев к статьям
- Медиа файлов (изображения)

## Сущности

### 1. Post (Статья)

```go
// domain/post.go
type Post struct {
    ID          string            `json:"id"`
    Title       string            `json:"title"`
    Slug        string            `json:"slug"`         // URL-friendly версия заголовка
    Content     string            `json:"content"`     // HTML контент
    Excerpt     string            `json:"excerpt"`     // Краткое описание
    AuthorID    string            `json:"author_id"`
    Status      string            `json:"status"`       // draft, published, archived
    CategoryID  *string           `json:"category_id,omitempty"`
    Tags        []string          `json:"tags"`
    FeaturedImage string          `json:"featured_image,omitempty"`
    PublishedAt *time.Time        `json:"published_at,omitempty"`
    Metadata    map[string]string  `json:"metadata,omitempty"`
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
    Description string    `json:"description,omitempty"`
    ParentID    *string   `json:"parent_id,omitempty"`  // Для вложенных категорий
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}
```

### 3. Comment (Комментарий)

```go
// domain/comment.go
type Comment struct {
    ID        string     `json:"id"`
    PostID    string     `json:"post_id"`
    AuthorName string    `json:"author_name"`
    AuthorEmail string   `json:"author_email"`
    Content   string     `json:"content"`
    Status    string     `json:"status"`  // pending, approved, spam, deleted
    ParentID  *string    `json:"parent_id,omitempty"`  // Для ответов на комментарии
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}
```

## Реализация

### Шаг 1: Domain

Создайте файлы в `domain/`:
- `post.go`
- `category.go`
- `comment.go`

### Шаг 2: Repository

```go
// repository/post.go
type PostRepository interface {
    GetByID(ctx context.Context, id string) (*domain.Post, error)
    GetBySlug(ctx context.Context, slug string) (*domain.Post, error)
    List(ctx context.Context, filter PostFilter) ([]domain.Post, error)
    Create(ctx context.Context, post *domain.Post) (*domain.Post, error)
    Update(ctx context.Context, post *domain.Post) error
    Delete(ctx context.Context, id string) error
}

type PostFilter struct {
    Status     string
    CategoryID string
    Tag        string
    AuthorID   string
    Search     string
    Limit      int
    Offset     int
}
```

### Шаг 3: Use Case

```go
// usecase/post/post.go
func (uc *UseCase) CreatePost(ctx context.Context, post *domain.Post) (*domain.Post, error) {
    // Генерация slug из заголовка
    if post.Slug == "" {
        post.Slug = generateSlug(post.Title)
    }
    
    // Установка статуса
    if post.Status == "" {
        post.Status = "draft"
    }
    
    // Если публикуется, устанавливаем дату публикации
    if post.Status == "published" && post.PublishedAt == nil {
        now := time.Now()
        post.PublishedAt = &now
    }
    
    return uc.repo.Create(ctx, post)
}

func generateSlug(title string) string {
    // Простая генерация slug (можно использовать библиотеку)
    slug := strings.ToLower(title)
    slug = strings.ReplaceAll(slug, " ", "-")
    return slug
}
```

### Шаг 4: Handler

```go
// api/handler/post.go
func (h *PostHandler) GetPostBySlug(ctx *fasthttp.RequestCtx) {
    slug := ctx.UserValue("slug").(string)
    
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()
    
    post, err := h.uc.GetPostBySlug(stdCtx, slug)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    h.respondSuccess(ctx, http.StatusOK, post)
}

func (h *PostHandler) ListPosts(ctx *fasthttp.RequestCtx) {
    filter := repository.PostFilter{
        Status:     string(ctx.QueryArgs().Peek("status")),
        CategoryID: string(ctx.QueryArgs().Peek("category_id")),
        Tag:        string(ctx.QueryArgs().Peek("tag")),
        Limit:      parseInt(string(ctx.QueryArgs().Peek("limit")), 10),
        Offset:     parseInt(string(ctx.QueryArgs().Peek("offset")), 0),
    }
    
    stdCtx, cancel := h.requestContext(ctx)
    defer cancel()
    
    posts, err := h.uc.ListPosts(stdCtx, filter)
    if err != nil {
        h.respondError(ctx, err)
        return
    }
    
    h.respondSuccess(ctx, http.StatusOK, posts)
}
```

## Миграции БД

```sql
-- assets/migrations/009_blog_posts.sql
CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    content TEXT NOT NULL,
    excerpt TEXT,
    author_id UUID NOT NULL REFERENCES users(id),
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    category_id UUID REFERENCES categories(id),
    tags TEXT[],  -- Массив тегов
    featured_image VARCHAR(255),
    published_at TIMESTAMP WITH TIME ZONE,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_posts_slug ON posts(slug);
CREATE INDEX idx_posts_status ON posts(status);
CREATE INDEX idx_posts_author_id ON posts(author_id);
CREATE INDEX idx_posts_category_id ON posts(category_id);
CREATE INDEX idx_posts_published_at ON posts(published_at);

-- assets/migrations/010_blog_categories.sql
CREATE TABLE categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    description TEXT,
    parent_id UUID REFERENCES categories(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_categories_slug ON categories(slug);
CREATE INDEX idx_categories_parent_id ON categories(parent_id);

-- assets/migrations/011_blog_comments.sql
CREATE TABLE comments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    author_name VARCHAR(255) NOT NULL,
    author_email VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    parent_id UUID REFERENCES comments(id),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_comments_post_id ON comments(post_id);
CREATE INDEX idx_comments_status ON comments(status);
CREATE INDEX idx_comments_parent_id ON comments(parent_id);
```

## API Endpoints

### Posts

- `GET /api/v1/posts` - Список статей (публичные)
- `GET /api/v1/posts/{slug}` - Получить статью по slug
- `POST /api/v1/posts` - Создать статью (требует авторизации)
- `PUT /api/v1/posts/{id}` - Обновить статью
- `DELETE /api/v1/posts/{id}` - Удалить статью

### Categories

- `GET /api/v1/categories` - Список категорий
- `POST /api/v1/categories` - Создать категорию

### Comments

- `GET /api/v1/posts/{id}/comments` - Комментарии к статье
- `POST /api/v1/posts/{id}/comments` - Добавить комментарий
- `PUT /api/v1/comments/{id}/approve` - Одобрить комментарий (админ)

## Примеры использования

### Получить опубликованные статьи

```bash
curl http://localhost:8080/api/v1/posts?status=published&limit=10
```

### Создать статью

```bash
curl -X POST http://localhost:8080/api/v1/posts \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Моя первая статья",
    "content": "<p>Содержание статьи...</p>",
    "excerpt": "Краткое описание",
    "status": "published",
    "category_id": "cat-123",
    "tags": ["go", "backend", "tutorial"]
  }'
```

### Добавить комментарий

```bash
curl -X POST http://localhost:8080/api/v1/posts/{post_id}/comments \
  -H "Content-Type: application/json" \
  -d '{
    "author_name": "Иван Иванов",
    "author_email": "ivan@example.com",
    "content": "Отличная статья!"
  }'
```

## Расширение функциональности

### Добавить медиа библиотеку

Создайте сущность `Media` для управления изображениями и файлами.

### Добавить SEO метаданные

Добавьте поля в `Post`:
- `meta_title`
- `meta_description`
- `meta_keywords`

### Добавить рейтинг статей

Создайте сущность `Rating` для оценки статей читателями.

## Следующие шаги

- [Мебельный магазин](./furniture-store.md) - Другой пример CMS
- [Общая документация CMS](../cms/README.md)

