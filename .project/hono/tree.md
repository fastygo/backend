# Hono Gateway Directory Structure

```
hono-gateway/
├── src/
│   ├── index.ts                    # Main Hono application
│   ├── server.ts                   # Server startup configuration
│   ├── middleware/
│   │   ├── auth.ts                 # JWT authentication middleware
│   │   ├── rateLimit.ts            # Rate limiting middleware
│   │   ├── cache.ts                # Redis caching middleware
│   │   ├── security.ts             # Security headers middleware
│   │   └── logging.ts              # Custom logging middleware
│   ├── routes/
│   │   ├── auth.ts                 # Authentication routes
│   │   ├── users.ts                # User management routes
│   │   ├── tasks.ts                # Task management routes
│   │   ├── admin.ts                # Admin routes
│   │   └── api.ts                  # API route aggregator
│   ├── services/
│   │   ├── backendProxy.ts         # Go backend proxy service
│   │   ├── supabaseService.ts      # Supabase client service
│   │   ├── redisService.ts         # Redis utility service
│   │   ├── validation.ts           # Input validation service
│   │   ├── sessionBuffer.ts        # Session storage buffer service
│   │   ├── networkMonitor.ts       # Network connectivity monitor
│   │   ├── bufferManager.ts        # Request buffer management
│   │   └── apiClient.ts            # Enhanced API client with buffering
│   ├── types/
│   │   ├── api.ts                  # API type definitions
│   │   ├── auth.ts                 # Authentication types
│   │   ├── user.ts                 # User types
│   │   ├── task.ts                 # Task types
│   │   └── common.ts               # Common types
│   ├── utils/
│   │   ├── errors.ts               # Error utilities
│   │   ├── responses.ts            # Response utilities
│   │   └── helpers.ts              # Helper functions
│   ├── hooks/
│   │   └── useOfflineStatus.ts     # React hook for offline status
│   └── components/
│       └── OfflineIndicator.tsx    # UI component for offline status
│   └── __tests__/
│       ├── middleware.test.ts
│       ├── routes.test.ts
│       ├── services.test.ts
│       └── integration.test.ts
├── config/
│   ├── development.ts              # Development configuration
│   ├── production.ts               # Production configuration
│   ├── test.ts                     # Test configuration
│   └── index.ts                    # Configuration loader
├── assets/
│   ├── migrations/                 # Database migrations (for Supabase)
│   │   ├── 001_initial.sql
│   │   └── 002_indexes.sql
│   └── seeds/                      # Seed data
│       └── development.sql
├── scripts/
│   ├── build.sh                    # Build script
│   ├── deploy.sh                   # Deployment script
│   ├── migrate.sh                  # Migration script
│   └── test.sh                     # Test script
├── Dockerfile                      # Docker build configuration
├── docker-compose.yml              # Local development setup
├── docker-compose.test.yml         # Testing environment
├── package.json                    # Node.js dependencies
├── package-lock.json               # Dependency lock file
├── tsconfig.json                   # TypeScript configuration
├── jest.config.js                  # Jest testing configuration
├── eslint.config.js                # ESLint configuration
├── .env.example                    # Environment variables template
├── .gitignore                      # Git ignore patterns
├── README.md                       # Project documentation
└── .dockerignore                   # Docker ignore patterns
```

## Key Architecture Notes

### Layer Responsibilities

1. **src/index.ts** - Main application setup, global middleware
2. **src/server.ts** - Server configuration and startup
3. **middleware/** - Hono middleware for cross-cutting concerns
   - **auth.ts** - JWT validation and user extraction
   - **rateLimit.ts** - Request throttling using Redis
   - **cache.ts** - Response caching with Redis
   - **security.ts** - OWASP security headers
   - **logging.ts** - Structured request logging
4. **routes/** - Route definitions and handlers
   - **auth.ts** - Login, logout, refresh, profile
   - **users.ts** - User CRUD operations
   - **tasks.ts** - Task management
   - **admin.ts** - Administrative functions
   - **api.ts** - Route aggregation and mounting
5. **services/** - Business logic and external integrations
   - **backendProxy.ts** - Proxy requests to Go backend
   - **supabaseService.ts** - Supabase database operations
   - **redisService.ts** - Redis caching and session management
   - **validation.ts** - Input validation using Zod
6. **types/** - TypeScript type definitions
7. **utils/** - Utility functions and helpers
8. **__tests__/** - Unit and integration tests

### Configuration Structure

```
config/
├── index.ts                        # Environment-based config loader
├── development.ts                  # Development settings
├── production.ts                   # Production settings
└── test.ts                         # Test settings
```

### Dual Mode Architecture

#### Go Backend Mode
```
Frontend → Hono Gateway → Go Backend
                    ↓
                 Redis Cache
```

#### Supabase Mode
```
Frontend → Hono Gateway → Supabase
                    ↓
                 Redis Cache
```

### Middleware Order

1. **CORS** - Handle cross-origin requests
2. **Logger** - Log incoming requests
3. **Rate Limit** - Throttle requests
4. **Security Headers** - Add security headers
5. **Authentication** - JWT validation
6. **Authorization** - Role-based access control
7. **Caching** - Response caching
8. **Routes** - Business logic handlers

### Environment Variables

```env
# Server
PORT=3000
NODE_ENV=development

# Mode Selection
MODE=go_backend  # or 'supabase'

# Go Backend Mode
BACKEND_URL=http://localhost:8080

# Supabase Mode
SUPABASE_URL=https://project.supabase.co
SUPABASE_ANON_KEY=anon-key
SUPABASE_SERVICE_ROLE_KEY=service-key

# Shared
REDIS_URL=redis://localhost:6379
JWT_SECRET=your-secret-key

# Security
CORS_ORIGIN=http://localhost:3000
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=900000

# Logging
LOG_LEVEL=info
```

### Docker Multi-Stage Build

```dockerfile
# Development stage
FROM node:18-alpine AS development
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "run", "dev"]

# Build stage
FROM development AS builder
RUN npm run build

# Production stage
FROM node:18-alpine AS production
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY --from=builder /app/dist ./dist
EXPOSE 3000
CMD ["npm", "start"]
```

### Development Setup

```yaml
# docker-compose.yml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data

  hono-gateway:
    build:
      context: .
      target: development
    ports:
      - "3000:3000"
    environment:
      MODE: go_backend
      BACKEND_URL: http://host.docker.internal:8080
      REDIS_URL: redis://redis:6379
    volumes:
      - .:/app
      - /app/node_modules
    depends_on:
      - redis

volumes:
  redis_data:
```

### Testing Structure

```typescript
// __tests__/integration.test.ts
import { testClient } from 'hono/testing'
import app from '../src/index'

describe('Hono Gateway Integration Tests', () => {
  const client = testClient(app)

  describe('Health Check', () => {
    test('returns healthy status', async () => {
      const res = await client.health.$get()
      expect(res.status).toBe(200)

      const data = await res.json()
      expect(data).toHaveProperty('status', 'ok')
      expect(data).toHaveProperty('mode')
    })
  })

  describe('Authentication', () => {
    test('login endpoint exists', async () => {
      const res = await client.api.v1.auth.login.$get()
      expect(res.status).toBe(405) // Method not allowed (expects POST)
    })

    test('protected route requires auth', async () => {
      const res = await client.api.v1.users.profile.$get()
      expect(res.status).toBe(401) // Unauthorized
    })
  })
})
```

### Deployment Options

#### Vercel (Serverless)
```javascript
// vercel.json
{
  "version": 2,
  "builds": [
    {
      "src": "src/server.ts",
      "use": "@vercel/node"
    }
  ],
  "routes": [
    {
      "src": "/(.*)",
      "dest": "src/server.ts"
    }
  ]
}
```

#### Cloudflare Workers
```typescript
// wrangler.toml
name = "hono-gateway"
main = "src/server.ts"
compatibility_date = "2023-12-01"

[vars]
MODE = "supabase"
SUPABASE_URL = "https://project.supabase.co"
JWT_SECRET = "your-secret"

[[kv_namespaces]]
binding = "CACHE"
id = "your-kv-namespace-id"
```

### Offline Resilience Architecture

#### Client-Side Data Flow
```
User Action → API Client → Network Check → Online?
    ↓              ↓              ↓          ↓
Immediate UI    Buffer Check    Yes       Execute Request
Update         (if offline)     No        Buffer Request
```

#### Buffer Processing Flow
```
Network Online → Buffer Manager → Sort by Priority → Process Queue
       ↓              ↓                ↓             ↓
   Status Change   Read Buffer     Critical First   Execute Requests
   Detected        from Session    Operations       Handle Failures
```

#### Key Components
1. **Session Buffer**: Browser session storage for offline request queuing
2. **Network Monitor**: Real-time connectivity detection and status tracking
3. **Buffer Manager**: Background service for processing buffered requests
4. **API Client**: Enhanced HTTP client with automatic buffering
5. **React Hooks**: UI integration for offline status and buffer state

#### Buffer Data Structure
```typescript
interface BufferedRequest {
  id: string              // Unique request ID
  url: string             // Target endpoint
  method: string          // HTTP method
  headers: Record<string,string>  // Request headers
  body?: any              // Request payload
  timestamp: number       // When buffered
  retries: number         // Retry attempts
  priority: number        // Processing priority (1-5)
  userId?: string         // User context
}
```

#### Configuration Options
- **SESSION_BUFFER_KEY**: Session storage key (default: 'offline_requests')
- **SYNC_INTERVAL_MS**: Background sync frequency (default: 30s)
- **MAX_BUFFER_SIZE**: Maximum buffered requests (default: 1000)
- **RETRY_ATTEMPTS**: Max retry attempts per request (default: 3)
- **CONNECTION_CHECK_INTERVAL_MS**: Connectivity check frequency (default: 5s)

#### UI Integration Patterns
```typescript
// Status Indicator Component
function OfflineIndicator() {
  const { isOnline, bufferSize } = useOfflineStatus()

  return (
    <div className={`status ${isOnline ? 'online' : 'offline'}`}>
      {isOnline ? '🟢 Online' : '🔴 Offline'}
      {bufferSize > 0 && ` (${bufferSize} pending)`}
    </div>
  )
}

// Optimistic Updates
function TaskList() {
  const [tasks, setTasks] = useState([])
  const { isOnline } = useOfflineStatus()

  const addTask = async (task) => {
    // Optimistic UI update
    const optimisticTask = { ...task, id: 'temp-' + Date.now() }
    setTasks(prev => [...prev, optimisticTask])

    try {
      await apiClient.post('/api/tasks', task, {
        bufferOnFailure: true,
        priority: 4
      })
    } catch (error) {
      // Revert optimistic update on failure
      setTasks(prev => prev.filter(t => t.id !== optimisticTask.id))
    }
  }
}
```

### Monitoring & Observability

- **Request Logging**: JSON structured logs with request IDs
- **Error Tracking**: Centralized error handling and reporting
- **Health Checks**: `/health` endpoint for load balancer
- **Metrics**: Response times, error rates, throughput
- **Distributed Tracing**: Request tracing across services

### Security Best Practices

1. **Input Validation**: Zod schemas for all inputs
2. **Rate Limiting**: Redis-based request throttling
3. **CORS**: Configured origins only
4. **Security Headers**: OWASP recommended headers
5. **JWT Security**: Secure secrets, token expiration
6. **HTTPS Only**: Enforce SSL/TLS
7. **No Secrets in Logs**: Sanitize sensitive data
8. **Dependency Updates**: Regular security updates
