# FastGo Backend

Modular Go backend with Clean Architecture for building flexible and scalable API services.

## 🚀 Features

- ✅ **Clean Architecture** - Separation into layers (Domain, UseCase, Repository, API)
- ✅ **High Performance** - Using fasthttp for minimal latency
- ✅ **Offline Resilience** - Automatic buffering of operations when DB is unavailable
- ✅ **Modularity** - Easy addition of new modules without changing the core
- ✅ **Production Ready** - Docker, health checks, graceful shutdown
- ✅ **Scalability** - Support for horizontal and vertical scaling

## 📚 Documentation

Full documentation is available in the [`docs/`](./docs/README.md) directory:

- **[Quick Start](./docs/quickstart.md)** - Get started in 5 minutes
- **[Architecture](./docs/architecture/README.md)** - Project structure and design principles
- **[Development](./docs/development/README.md)** - Development guide
- **[Deployment](./docs/deployment/README.md)** - Deployment instructions
- **[Usage Examples](./docs/examples/README.md)** - Ready-to-use scenarios for different application types

## 🛠 Tech Stack

- **Language**: Go 1.21+
- **HTTP Server**: fasthttp
- **Database**: PostgreSQL (pgx/v5)
- **Cache/Sessions**: Redis
- **Buffer**: BoltDB (for offline operations)
- **Logging**: Zap
- **Migrations**: golang-migrate

## ⚡ Quick Start

```bash
# Clone repository
git clone <repo-url> backend
cd backend

# Install dependencies
go mod tidy

# Setup environment
cp .env.example .env
# Edit .env file with your settings

# Run with Docker Compose
docker-compose up -d

# Or run locally
make run
```

Check if it's working:

```bash
curl http://localhost:8080/health
```

## 📖 Usage Examples

The project is ready for various types of applications:

- **CRM Systems** - [Web Studio](./docs/examples/crm/webstudio.md), [Coffee Shop](./docs/examples/crm/coffee-shop.md)
- **CMS Systems** - [Blog](./docs/examples/cms/blog.md), [Furniture Store](./docs/examples/cms/furniture-store.md)
- **Chats** - [Simple Chat](./docs/examples/chat/simple.md), [Chat with Roles](./docs/examples/chat/with-roles.md)
- **Dashboards** - [Project and Task Management](./docs/examples/dashboard/README.md)

## 🏗 Project Structure

```
backend/
├── cmd/server/          # Application entry point
├── internal/            # Internal code
│   ├── config/         # Configuration
│   ├── infrastructure/  # DB, Redis connections
│   ├── middleware/     # HTTP middleware
│   └── router/         # Routing
├── api/                 # HTTP interface
├── usecase/            # Business logic
├── repository/         # Data access
├── domain/             # Domain entities
└── docs/               # Documentation
```

## 🔧 Main Commands

| Command | Description |
|---------|------------|
| `make build` | Build the project |
| `make run` | Run the server |
| `make test` | Run tests |
| `make lint` | Check code |
| `make docker-build` | Build Docker image |

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a PR.

## 📞 Support

If you have any questions:

1. Check the [documentation](./docs/README.md)
2. Review the [examples](./docs/examples/README.md)
3. Create an issue in the repository

---

**Made with ❤️ for Go developers**
