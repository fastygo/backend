# Hono Gateway Plan: Universal API Gateway

## Overview
Lightweight, fast API Gateway using Hono framework. Acts as a universal intermediary between frontend applications and backend services (Go backend or Supabase). Handles authentication, request routing, caching, and middleware.

## Core Requirements
- **Performance**: Edge-compatible runtime (Cloudflare Workers, Vercel, Node.js)
- **Flexibility**: Connect to Go backend OR Supabase seamlessly
- **Security**: JWT validation, CORS, rate limiting, security headers
- **Observability**: Request logging, error tracking, metrics
- **Production Ready**: Docker, health checks, graceful error handling
- **Offline Resilience**: Session storage buffering, connection recovery, data persistence

## Offline Resilience & Data Buffering

### Session Storage as Default Buffer
- **Browser Session Storage**: Client-side data persistence during network issues
- **Automatic Retry**: Failed requests automatically retried when connection restored
- **Queue Management**: Request queuing with priority and deduplication
- **Conflict Resolution**: Timestamp-based conflict resolution for concurrent updates
- **Data Synchronization**: Smart merging of offline changes with server state

### Buffer Strategy
- **Immediate Response**: Optimistic UI updates with local storage
- **Background Sync**: Automatic synchronization when connectivity returns
- **Priority Queue**: Critical operations (auth, saves) processed first
- **Batch Processing**: Multiple requests batched for efficiency
- **Offline Indicators**: UI feedback for offline/online status

### Connection Recovery
- **Network Detection**: Automatic detection of connectivity changes
- **Exponential Backoff**: Smart retry logic for failed requests
- **Circuit Breaker**: Prevent cascade failures during outages
- **Graceful Degradation**: Fallback behavior when services unavailable

## Architecture Modes

### Mode 1: Go Backend Proxy
```
React → Hono Gateway → Go Backend (fasthttp)
                    ↓
               Redis (sessions/cache)
```

### Mode 2: Supabase Direct
```
React → Hono Gateway → Supabase (PostgreSQL + Auth)
                    ↓
               Redis (cache only)
```

## API Routes Structure
```
/api/v1/
├── auth/
│   ├── login          # POST - authenticate user
│   ├── logout         # POST - clear session
│   ├── refresh        # POST - refresh tokens
│   └── me             # GET - current user profile
├── users/             # User management
│   ├── profile        # GET/PUT - user profile
│   └── preferences    # GET/PUT - user settings
├── tasks/             # Task management
│   ├── {id}           # GET/PUT/DELETE - single task
│   └── search         # GET - search tasks
└── admin/             # Admin operations
    ├── users          # GET - list users
    └── stats          # GET - system statistics
```

## Technology Stack
```json
{
  "hono": "^4.0.0",
  "hono/jwt": "^1.0.0",
  "hono/cors": "^1.0.0",
  "hono/logger": "^1.0.0",
  "redis": "^4.6.0",
  "@supabase/supabase-js": "^2.38.0",
  "zod": "^3.22.0"
}
```

## Environment Configurations

### For Go Backend Mode
```env
MODE=go_backend
BACKEND_URL=http://backend:8080
REDIS_URL=redis://redis:6379
JWT_SECRET=your-secret-key
SUPABASE_URL=  # not used
SUPABASE_ANON_KEY=  # not used
```

### For Supabase Mode
```env
MODE=supabase
BACKEND_URL=  # not used
REDIS_URL=redis://redis:6379
JWT_SECRET=your-secret-key
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_ANON_KEY=your-anon-key
SUPABASE_SERVICE_ROLE_KEY=your-service-key
```

## Security Features
- JWT token validation and refresh
- CORS configuration for frontend origins
- Rate limiting per IP/user
- Request sanitization and validation
- Security headers (HSTS, CSP, X-Frame-Options)
- SQL injection prevention through parameterized queries

## Middleware Stack
1. **Request ID**: Unique identifier for request tracing
2. **Logging**: Structured JSON logging
3. **CORS**: Cross-origin resource sharing
4. **Rate Limiting**: Request throttling
5. **Security Headers**: OWASP security headers
6. **Authentication**: JWT validation
7. **Authorization**: Role-based access control
8. **Caching**: Response caching with Redis
9. **Error Handling**: Centralized error responses

## Caching Strategy
- **Static Data**: Cache for 1 hour (user profiles, settings)
- **Dynamic Data**: Cache for 5 minutes (task lists, search results)
- **Real-time Data**: No cache (immediate updates)
- **Cache Keys**: Include user ID and request parameters
- **Cache Invalidation**: On data mutations

## Error Handling
- **4xx Errors**: Client errors (validation, authentication)
- **5xx Errors**: Server errors (backend failures)
- **Timeout Handling**: 30-second timeout for backend calls
- **Circuit Breaker**: Prevent cascading failures
- **Fallback Responses**: Graceful degradation

## Monitoring & Observability
- Request/response logging
- Error tracking and alerting
- Performance metrics (response times, error rates)
- Health check endpoints
- Distributed tracing support

## Deployment Options
- **Cloudflare Workers**: Global edge deployment
- **Vercel**: Serverless functions
- **Docker**: Containerized deployment
- **Node.js**: Traditional server deployment

## GDPR Compliance
- No personal data storage (PII in IAM/Supabase only)
- Request logging without sensitive data
- Data minimization principles
- User consent handling
- Right to erasure support
