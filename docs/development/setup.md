# Настройка окружения

Пошаговая инструкция по настройке окружения для разработки.

## Требования

- **Go 1.21+** - Язык программирования
- **PostgreSQL 15+** - База данных
- **Redis 7+** - Кэш и сессии
- **Docker & Docker Compose** - Для локальной разработки (опционально)
- **Make** - Для удобных команд (опционально)

## Установка Go

### Windows

1. Скачайте установщик с [golang.org](https://golang.org/dl/)
2. Запустите установщик
3. Проверьте установку:
```bash
go version
```

### Linux

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install golang-go

# Проверка
go version
```

### macOS

```bash
# Используя Homebrew
brew install go

# Проверка
go version
```

## Настройка Go окружения

### Переменные окружения

Добавьте в `~/.bashrc` или `~/.zshrc`:

```bash
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
```

### Проверка

```bash
go env GOPATH
go env GOROOT
```

## Установка PostgreSQL

### Windows

1. Скачайте установщик с [postgresql.org](https://www.postgresql.org/download/windows/)
2. Запустите установщик
3. Запомните пароль для пользователя `postgres`

### Linux

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install postgresql postgresql-contrib

# Запуск
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

### macOS

```bash
brew install postgresql
brew services start postgresql
```

### Создание базы данных

```bash
# Подключение
psql -U postgres

# Создание базы и пользователя
CREATE DATABASE backend_db;
CREATE USER backend_user WITH PASSWORD 'backend_pass';
GRANT ALL PRIVILEGES ON DATABASE backend_db TO backend_user;
\q
```

## Установка Redis

### Windows

Используйте WSL или Docker (рекомендуется).

### Linux

```bash
# Ubuntu/Debian
sudo apt update
sudo apt install redis-server

# Запуск
sudo systemctl start redis
sudo systemctl enable redis
```

### macOS

```bash
brew install redis
brew services start redis
```

### Проверка

```bash
redis-cli ping
# Должно вернуть: PONG
```

## Установка Docker (опционально)

### Windows

1. Скачайте Docker Desktop с [docker.com](https://www.docker.com/products/docker-desktop)
2. Установите и запустите

### Linux

```bash
# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER
```

### macOS

```bash
brew install --cask docker
```

## Настройка проекта

### 1. Клонирование

```bash
git clone <repo-url> backend
cd backend
```

### 2. Установка зависимостей

```bash
go mod download
go mod tidy
```

### 3. Настройка переменных окружения

```bash
cp .env.example .env
```

Отредактируйте `.env`:

```env
APP_NAME=go-backend
APP_ENV=development
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

DB_HOST=localhost
DB_PORT=5432
DB_NAME=backend_db
DB_USER=backend_user
DB_PASSWORD=backend_pass

REDIS_URL=redis://localhost:6379
JWT_SECRET=your-secret-key-here

BOLTDB_PATH=./data/buffer.db
BUFFER_MAX_SIZE=1000000
BUFFER_RETENTION_HOURS=24
SYNC_INTERVAL_SECONDS=30
MAX_RETRY_ATTEMPTS=3
```

### 4. Запуск миграций

```bash
# Если используете Docker Compose
docker-compose up -d postgres
make migrate

# Или вручную
psql -U backend_user -d backend_db -f assets/migrations/001_initial.sql
```

### 5. Запуск проекта

```bash
# Локально
make run

# Или через Docker Compose
docker-compose up
```

## Проверка установки

### 1. Проверка Go

```bash
go version
# Должно показать: go version go1.21.x ...
```

### 2. Проверка PostgreSQL

```bash
psql -U backend_user -d backend_db -c "SELECT version();"
```

### 3. Проверка Redis

```bash
redis-cli ping
# Должно вернуть: PONG
```

### 4. Проверка проекта

```bash
# Сборка
make build

# Тесты
make test

# Запуск
make run
```

Откройте в браузере: `http://localhost:8080/health`

Должен вернуться JSON с информацией о здоровье сервиса.

## Настройка IDE

### VS Code

1. Установите расширение "Go"
2. Настройки (`.vscode/settings.json`):
```json
{
    "go.useLanguageServer": true,
    "go.formatTool": "goimports",
    "go.lintTool": "golangci-lint"
}
```

### GoLand

1. Откройте проект
2. Go → Go Modules → Enable Go Modules
3. File → Settings → Go → Build Tags & Vendoring

## Решение проблем

### Проблема: `go: command not found`

**Решение**: Добавьте Go в PATH:
```bash
export PATH=$PATH:/usr/local/go/bin
```

### Проблема: `cannot connect to PostgreSQL`

**Решение**: Проверьте:
1. PostgreSQL запущен: `sudo systemctl status postgresql`
2. Правильные credentials в `.env`
3. База данных создана

### Проблема: `cannot connect to Redis`

**Решение**: Проверьте:
1. Redis запущен: `redis-cli ping`
2. Правильный URL в `.env`

### Проблема: порт уже занят

**Решение**: Измените `SERVER_PORT` в `.env` или остановите процесс:
```bash
# Linux/macOS
lsof -i :8080
kill -9 <PID>

# Windows
netstat -ano | findstr :8080
taskkill /PID <PID> /F
```

## Следующие шаги

- [Структура кода](./code-structure.md)
- [Добавление новой функциональности](./adding-features.md)

