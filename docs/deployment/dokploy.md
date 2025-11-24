# Развертывание на Dokploy

Пошаговое руководство по развертыванию бэкенда на Dokploy - простой платформе для деплоя приложений.

## Что такое Dokploy?

Dokploy - это open-source альтернатива платформам вроде Vercel или Netlify, но для ваших собственных серверов. Она позволяет легко развертывать приложения через Docker.

## Предварительные требования

1. **Сервер** с Ubuntu 20.04+ или Debian 11+
2. **Docker и Docker Compose** установлены на сервере
3. **Доступ по SSH** к серверу
4. **Домен** (опционально, но рекомендуется)

## Шаг 1: Установка Dokploy

### На сервере выполните:

```bash
# Обновление системы
sudo apt update && sudo apt upgrade -y

# Установка Docker (если еще не установлен)
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Установка Docker Compose
sudo apt install docker-compose-plugin -y

# Установка Dokploy
curl -L https://dokploy.com/install.sh | bash
```

После установки Dokploy будет доступен по адресу: `http://your-server-ip:3000`

## Шаг 2: Первоначальная настройка Dokploy

1. Откройте браузер и перейдите на `http://your-server-ip:3000`
2. Создайте администратора
3. Настройте базу данных (Dokploy использует SQLite по умолчанию, можно настроить PostgreSQL)

## Шаг 3: Подготовка проекта

### На вашем локальном компьютере:

1. **Убедитесь, что проект готов к деплою**:
```bash
# Проверьте, что все работает локально
make test
make build
```

2. **Создайте `.dockerignore`** (если еще не создан):
```
.git
.gitignore
.env
.env.local
*.md
docs/
.vscode/
.idea/
```

3. **Проверьте `Dockerfile`**:
```dockerfile
# Должен быть многоступенчатый build
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/main .
COPY --from=builder /app/assets ./assets
EXPOSE 8080
CMD ["./main"]
```

## Шаг 4: Настройка базы данных в Dokploy

### Вариант 1: Использовать встроенный PostgreSQL от Dokploy

1. В Dokploy перейдите в **Databases**
2. Создайте новую базу данных PostgreSQL
3. Запомните credentials (host, port, user, password, database name)

### Вариант 2: Использовать внешний PostgreSQL

Если у вас уже есть PostgreSQL на другом сервере, используйте его credentials.

## Шаг 5: Настройка Redis в Dokploy

1. В Dokploy перейдите в **Databases**
2. Создайте новую базу данных Redis
3. Запомните connection string

## Шаг 6: Создание приложения в Dokploy

1. **Создайте новый проект**:
   - Нажмите "New Project"
   - Введите название (например, "backend")
   - Выберите тип: "Docker"

2. **Настройте Git репозиторий**:
   - Выберите "Git Repository"
   - Введите URL вашего репозитория
   - Выберите ветку (обычно `main` или `master`)
   - Настройте доступ (если репозиторий приватный)

3. **Настройте Dockerfile**:
   - Путь к Dockerfile: `./Dockerfile`
   - Build context: `.`

## Шаг 7: Настройка переменных окружения

В настройках приложения добавьте переменные окружения:

```env
APP_NAME=go-backend
APP_ENV=production
SERVER_HOST=0.0.0.0
SERVER_PORT=8080

DB_HOST=<postgres-host-from-dokploy>
DB_PORT=5432
DB_NAME=<database-name>
DB_USER=<database-user>
DB_PASSWORD=<database-password>

REDIS_URL=redis://<redis-host>:6379
JWT_SECRET=<generate-strong-secret-here>

BOLTDB_PATH=/data/buffer.db
BUFFER_MAX_SIZE=1000000
BUFFER_RETENTION_HOURS=24
SYNC_INTERVAL_SECONDS=30
MAX_RETRY_ATTEMPTS=3
```

**Важно**: 
- Используйте сильный `JWT_SECRET` (можно сгенерировать: `openssl rand -base64 32`)
- `DB_HOST` и `REDIS_URL` должны указывать на базы данных из Dokploy

## Шаг 8: Настройка портов и сети

1. **Порт приложения**: `8080`
2. **Публичный порт**: Выберите свободный порт (например, `3001`)
3. **Health check**: `/health`

## Шаг 9: Настройка volumes (для BoltDB)

Добавьте volume для сохранения данных буфера:

- **Host path**: `/var/lib/dokploy/data/backend-buffer`
- **Container path**: `/data`

Это позволит сохранять данные буфера между перезапусками.

## Шаг 10: Деплой

1. Нажмите **"Deploy"**
2. Dokploy начнет сборку Docker образа
3. После успешной сборки приложение запустится

## Шаг 11: Проверка

1. **Проверьте логи**:
   - В Dokploy перейдите в раздел "Logs"
   - Убедитесь, что нет ошибок

2. **Проверьте health endpoint**:
```bash
curl http://your-server-ip:3001/health
```

Должен вернуться JSON с информацией о здоровье сервиса.

## Шаг 12: Настройка домена (опционально)

### Если у вас есть домен:

1. **Настройте DNS**:
   - Добавьте A-запись: `api.yourdomain.com` → IP вашего сервера

2. **В Dokploy**:
   - Перейдите в настройки приложения
   - Добавьте домен: `api.yourdomain.com`
   - Dokploy автоматически настроит reverse proxy

3. **Настройте SSL**:
   - В Dokploy включите "SSL"
   - Выберите "Let's Encrypt"
   - Dokploy автоматически получит сертификат

## Шаг 13: Настройка автоматического деплоя

### При push в репозиторий:

1. В настройках приложения включите **"Auto Deploy"**
2. Выберите ветку (обычно `main`)
3. Теперь при каждом push Dokploy автоматически пересоберет и перезапустит приложение

## Мониторинг и логи

### Просмотр логов:

1. В Dokploy перейдите в раздел "Logs"
2. Выберите ваше приложение
3. Просматривайте логи в реальном времени

### Метрики:

- Используйте endpoint `/health` для мониторинга
- Настройте внешний мониторинг (например, UptimeRobot)

## Обновление приложения

### Ручное обновление:

1. В Dokploy нажмите **"Redeploy"**
2. Приложение пересоберется и перезапустится

### Автоматическое обновление:

Если включен "Auto Deploy", просто сделайте push в репозиторий.

## Резервное копирование

### База данных:

1. В Dokploy перейдите в раздел "Databases"
2. Выберите вашу базу данных
3. Нажмите "Backup"
4. Скачайте backup файл

### Рекомендации:

- Настройте автоматические бэкапы
- Храните бэкапы в безопасном месте
- Тестируйте восстановление из бэкапа

## Решение проблем

### Проблема: Приложение не запускается

**Решение**:
1. Проверьте логи в Dokploy
2. Убедитесь, что все переменные окружения настроены
3. Проверьте, что базы данных доступны

### Проблема: Не могу подключиться к БД

**Решение**:
1. Проверьте credentials в переменных окружения
2. Убедитесь, что БД запущена в Dokploy
3. Проверьте сетевые настройки

### Проблема: Health check не проходит

**Решение**:
1. Проверьте, что приложение слушает на правильном порту
2. Убедитесь, что endpoint `/health` существует
3. Проверьте логи приложения

## Следующие шаги

- [Production Checklist](./production-checklist.md) - Чеклист для продакшена
- [Docker](./docker.md) - Детали Docker развертывания

