# Hono Gateway Guide: Universal API Gateway 101

## Prerequisites
- Node.js 18+
- npm or bun
- Redis 7+
- Go backend OR Supabase account

## Project Setup

### 1. Initialize Project
```bash
mkdir hono-gateway
cd hono-gateway
npm init -y
```

### 2. Install Dependencies
```bash
npm install hono @hono/jwt @hono/cors @hono/logger
npm install redis @supabase/supabase-js zod
npm install -D @types/node typescript tsx
```

### 3. Environment Configuration
Create `.env` file:
```env
PORT=3000
MODE=go_backend  # or 'supabase'
BACKEND_URL=http://localhost:8080
REDIS_URL=redis://localhost:6379
JWT_SECRET=your-super-secret-jwt-key
SUPABASE_URL=https://your-project.supabase.co
SUPABASE_ANON_KEY=your-anon-key
SUPABASE_SERVICE_ROLE_KEY=your-service-key
CORS_ORIGIN=http://localhost:3000
RATE_LIMIT_REQUESTS=100
RATE_LIMIT_WINDOW=900000  # 15 minutes

# Offline Resilience Configuration
SESSION_BUFFER_KEY=offline_requests
SYNC_INTERVAL_MS=30000
MAX_BUFFER_SIZE=1000
RETRY_ATTEMPTS=3
CONNECTION_CHECK_INTERVAL_MS=5000
```

## Offline Resilience & Session Storage Buffering

### Session Storage Buffer Service
```typescript
// src/services/sessionBuffer.ts
export interface BufferedRequest {
  id: string
  url: string
  method: string
  headers: Record<string, string>
  body?: any
  timestamp: number
  retries: number
  priority: number // 1=low, 5=critical
  userId?: string
}

export class SessionBuffer {
  private static readonly BUFFER_KEY = 'offline_requests'
  private static readonly MAX_SIZE = 1000
  private syncInProgress = false

  static getBuffer(): BufferedRequest[] {
    try {
      const stored = sessionStorage.getItem(this.BUFFER_KEY)
      return stored ? JSON.parse(stored) : []
    } catch {
      return []
    }
  }

  static setBuffer(buffer: BufferedRequest[]): void {
    try {
      // Keep only recent items if buffer is too large
      const trimmed = buffer
        .sort((a, b) => b.timestamp - a.timestamp)
        .slice(0, this.MAX_SIZE)

      sessionStorage.setItem(this.BUFFER_KEY, JSON.stringify(trimmed))
    } catch (error) {
      console.warn('Failed to save buffer to session storage:', error)
    }
  }

  static addRequest(request: Omit<BufferedRequest, 'id' | 'timestamp' | 'retries'>): void {
    const buffer = this.getBuffer()
    const newRequest: BufferedRequest = {
      ...request,
      id: crypto.randomUUID(),
      timestamp: Date.now(),
      retries: 0,
    }

    // Remove duplicates (same URL + method + body)
    const filtered = buffer.filter(req =>
      !(req.url === newRequest.url &&
        req.method === newRequest.method &&
        JSON.stringify(req.body) === JSON.stringify(newRequest.body))
    )

    filtered.push(newRequest)
    this.setBuffer(filtered)
  }

  static removeRequest(id: string): void {
    const buffer = this.getBuffer()
    const filtered = buffer.filter(req => req.id !== id)
    this.setBuffer(filtered)
  }

  static clearBuffer(): void {
    sessionStorage.removeItem(this.BUFFER_KEY)
  }

  static getBufferSize(): number {
    return this.getBuffer().length
  }
}
```

### Network Status Monitor
```typescript
// src/services/networkMonitor.ts
export type NetworkStatus = 'online' | 'offline' | 'unknown'

export class NetworkMonitor {
  private status: NetworkStatus = 'unknown'
  private listeners: ((status: NetworkStatus) => void)[] = []

  constructor() {
    this.checkStatus()
    this.setupListeners()
    // Periodic connectivity check
    setInterval(() => this.checkStatus(), 5000)
  }

  private setupListeners(): void {
    window.addEventListener('online', () => this.updateStatus('online'))
    window.addEventListener('offline', () => this.updateStatus('offline'))
  }

  private async checkStatus(): Promise<void> {
    try {
      // Check actual connectivity by making a request
      const response = await fetch('/health', {
        method: 'HEAD',
        cache: 'no-cache'
      })
      const newStatus = response.ok ? 'online' : 'offline'
      this.updateStatus(newStatus)
    } catch {
      this.updateStatus('offline')
    }
  }

  private updateStatus(newStatus: NetworkStatus): void {
    if (this.status !== newStatus) {
      this.status = newStatus
      this.listeners.forEach(listener => listener(newStatus))
    }
  }

  getStatus(): NetworkStatus {
    return this.status
  }

  onStatusChange(callback: (status: NetworkStatus) => void): () => void {
    this.listeners.push(callback)
    return () => {
      const index = this.listeners.indexOf(callback)
      if (index > -1) {
        this.listeners.splice(index, 1)
      }
    }
  }

  isOnline(): boolean {
    return this.status === 'online'
  }
}
```

### Request Buffer Manager
```typescript
// src/services/bufferManager.ts
import { SessionBuffer, BufferedRequest } from './sessionBuffer'
import { NetworkMonitor } from './networkMonitor'

export class BufferManager {
  private networkMonitor: NetworkMonitor
  private syncInterval: number
  private maxRetries: number

  constructor(networkMonitor: NetworkMonitor) {
    this.networkMonitor = networkMonitor
    this.syncInterval = parseInt(process.env.SYNC_INTERVAL_MS || '30000')
    this.maxRetries = parseInt(process.env.RETRY_ATTEMPTS || '3')

    this.startSyncProcess()
    this.networkMonitor.onStatusChange(status => {
      if (status === 'online') {
        this.processBuffer()
      }
    })
  }

  private startSyncProcess(): void {
    setInterval(() => {
      if (this.networkMonitor.isOnline() && !SessionBuffer['syncInProgress']) {
        this.processBuffer()
      }
    }, this.syncInterval)
  }

  async bufferRequest(
    url: string,
    method: string,
    headers: Record<string, string> = {},
    body?: any,
    priority: number = 3
  ): Promise<void> {
    SessionBuffer.addRequest({
      url,
      method,
      headers,
      body,
      priority,
      userId: this.getCurrentUserId()
    })
  }

  private async processBuffer(): Promise<void> {
    if (SessionBuffer['syncInProgress']) return

    SessionBuffer['syncInProgress'] = true

    try {
      const buffer = SessionBuffer.getBuffer()
      const criticalRequests = buffer.filter(req => req.priority >= 4)
      const regularRequests = buffer.filter(req => req.priority < 4)

      // Process critical requests first
      await this.processRequests([...criticalRequests, ...regularRequests])
    } finally {
      SessionBuffer['syncInProgress'] = false
    }
  }

  private async processRequests(requests: BufferedRequest[]): Promise<void> {
    for (const request of requests) {
      if (!this.networkMonitor.isOnline()) break

      try {
        const response = await this.executeRequest(request)

        if (response.ok) {
          SessionBuffer.removeRequest(request.id)
        } else {
          await this.handleFailedRequest(request)
        }
      } catch (error) {
        await this.handleFailedRequest(request)
      }
    }
  }

  private async executeRequest(request: BufferedRequest): Promise<Response> {
    const options: RequestInit = {
      method: request.method,
      headers: request.headers,
    }

    if (request.body && ['POST', 'PUT', 'PATCH'].includes(request.method)) {
      options.body = JSON.stringify(request.body)
    }

    return fetch(request.url, options)
  }

  private async handleFailedRequest(request: BufferedRequest): Promise<void> {
    request.retries++

    if (request.retries >= this.maxRetries) {
      SessionBuffer.removeRequest(request.id)
      console.warn(`Dropping failed request after ${this.maxRetries} retries:`, request.url)
    } else {
      // Update retries count in buffer
      const buffer = SessionBuffer.getBuffer()
      const index = buffer.findIndex(req => req.id === request.id)
      if (index > -1) {
        buffer[index] = request
        SessionBuffer.setBuffer(buffer)
      }
    }
  }

  private getCurrentUserId(): string | undefined {
    // Extract from JWT token or context
    try {
      const token = localStorage.getItem('auth_token')
      if (token) {
        const payload = JSON.parse(atob(token.split('.')[1]))
        return payload.sub
      }
    } catch {
      // Ignore parsing errors
    }
    return undefined
  }

  getBufferStatus() {
    return {
      size: SessionBuffer.getBufferSize(),
      syncing: SessionBuffer['syncInProgress'],
      networkStatus: this.networkMonitor.getStatus()
    }
  }
}
```

### Enhanced API Client with Buffering
```typescript
// src/services/apiClient.ts
import { BufferManager } from './bufferManager'
import { NetworkMonitor } from './networkMonitor'

export class ApiClient {
  private bufferManager: BufferManager
  private networkMonitor: NetworkMonitor

  constructor() {
    this.networkMonitor = new NetworkMonitor()
    this.bufferManager = new BufferManager(this.networkMonitor)
  }

  async request(
    url: string,
    method: string = 'GET',
    body?: any,
    headers: Record<string, string> = {},
    options: {
      bufferOnFailure?: boolean
      priority?: number
      skipAuth?: boolean
    } = {}
  ): Promise<Response> {
    const { bufferOnFailure = true, priority = 3, skipAuth = false } = options

    // Add authorization header if not skipped
    if (!skipAuth) {
      const token = localStorage.getItem('auth_token')
      if (token) {
        headers['Authorization'] = `Bearer ${token}`
      }
    }

    // Add other default headers
    headers['Content-Type'] = headers['Content-Type'] || 'application/json'

    try {
      const response = await fetch(url, {
        method,
        headers,
        body: body ? JSON.stringify(body) : undefined,
      })

      // Handle specific error codes
      if (response.status === 401) {
        // Token expired, redirect to login
        localStorage.removeItem('auth_token')
        window.location.href = '/login'
        throw new Error('Authentication required')
      }

      return response
    } catch (error) {
      if (bufferOnFailure && this.networkMonitor.getStatus() === 'offline') {
        // Buffer the request for later retry
        await this.bufferManager.bufferRequest(url, method, headers, body, priority)
        throw new Error('Request buffered - will retry when online')
      }

      throw error
    }
  }

  // Convenience methods
  async get(url: string, options?: Parameters<typeof this.request>[4]) {
    return this.request(url, 'GET', undefined, {}, options)
  }

  async post(url: string, body?: any, options?: Parameters<typeof this.request>[4]) {
    return this.request(url, 'POST', body, {}, options)
  }

  async put(url: string, body?: any, options?: Parameters<typeof this.request>[4]) {
    return this.request(url, 'PUT', body, {}, options)
  }

  async delete(url: string, options?: Parameters<typeof this.request>[4]) {
    return this.request(url, 'DELETE', undefined, {}, options)
  }

  getBufferStatus() {
    return this.bufferManager.getBufferStatus()
  }
}

// Global instance
export const apiClient = new ApiClient()
```

### React Hook for Offline Status
```typescript
// src/hooks/useOfflineStatus.ts
import { useState, useEffect } from 'react'
import { NetworkMonitor, NetworkStatus } from '../services/networkMonitor'

let networkMonitor: NetworkMonitor | null = null

export function useOfflineStatus() {
  const [status, setStatus] = useState<NetworkStatus>('unknown')
  const [bufferSize, setBufferSize] = useState(0)

  useEffect(() => {
    if (!networkMonitor) {
      networkMonitor = new NetworkMonitor()
    }

    setStatus(networkMonitor.getStatus())

    const unsubscribe = networkMonitor.onStatusChange((newStatus) => {
      setStatus(newStatus)
    })

    // Update buffer size periodically
    const updateBufferSize = () => {
      const buffer = JSON.parse(sessionStorage.getItem('offline_requests') || '[]')
      setBufferSize(buffer.length)
    }

    updateBufferSize()
    const interval = setInterval(updateBufferSize, 2000)

    return () => {
      unsubscribe()
      clearInterval(interval)
    }
  }, [])

  return {
    isOnline: status === 'online',
    isOffline: status === 'offline',
    status,
    bufferSize,
    hasBufferedRequests: bufferSize > 0,
  }
}
```

### Usage in Components
```typescript
// src/components/TaskForm.tsx
import { useOfflineStatus } from '../hooks/useOfflineStatus'
import { apiClient } from '../services/apiClient'

function TaskForm() {
  const { isOnline, bufferSize } = useOfflineStatus()

  const handleSubmit = async (taskData: any) => {
    try {
      const response = await apiClient.post('/api/v1/tasks', taskData, {
        bufferOnFailure: true,
        priority: 4, // High priority for saves
      })

      if (response.ok) {
        // Success - update UI immediately
        console.log('Task created successfully')
      }
    } catch (error) {
      if (error.message.includes('buffered')) {
        // Show offline notification
        console.log('Task saved offline - will sync when online')
      }
    }
  }

  return (
    <div>
      <form onSubmit={handleSubmit}>
        {/* Form fields */}
        <button type="submit" disabled={!isOnline && bufferSize > 10}>
          {isOnline ? 'Save Task' : `Save Offline (${bufferSize} pending)`}
        </button>
      </form>

      {!isOnline && (
        <div className="offline-notice">
          You're offline. Changes will be saved and synced when connection returns.
        </div>
      )}
    </div>
  )
}
```

## Basic Hono Application

### Entry Point
```typescript
// src/index.ts
import { Hono } from 'hono'
import { cors } from 'hono/cors'
import { logger } from 'hono/logger'
import { jwt } from 'hono/jwt'

const app = new Hono()

// Global middleware
app.use('*', cors({
  origin: process.env.CORS_ORIGIN || '*',
  allowMethods: ['GET', 'POST', 'PUT', 'DELETE'],
  allowHeaders: ['Content-Type', 'Authorization'],
}))

app.use('*', logger())

// Health check
app.get('/health', (c) => c.json({ status: 'ok', timestamp: new Date().toISOString() }))

export default app
```

## Authentication Middleware

### JWT Validation
```typescript
// src/middleware/auth.ts
import { jwt } from 'hono/jwt'
import { MiddlewareHandler } from 'hono'

export const jwtAuth = (): MiddlewareHandler => {
  return jwt({
    secret: process.env.JWT_SECRET!,
  })
}

// Optional: Extract user info from JWT
export const extractUser = (): MiddlewareHandler => {
  return async (c, next) => {
    const payload = c.get('jwtPayload')
    if (payload) {
      c.set('userId', payload.sub)
      c.set('userRole', payload.role)
    }
    await next()
  }
}
```

## Rate Limiting Middleware

```typescript
// src/middleware/rateLimit.ts
import { MiddlewareHandler } from 'hono'
import { Redis } from 'redis'

const redis = new Redis(process.env.REDIS_URL!)

export const rateLimit = (requests: number = 100, windowMs: number = 900000): MiddlewareHandler => {
  return async (c, next) => {
    const ip = c.req.header('CF-Connecting-IP') ||
               c.req.header('X-Forwarded-For') ||
               c.req.header('X-Real-IP') ||
               'unknown'

    const key = `rate_limit:${ip}`
    const now = Date.now()

    // Get current requests in window
    const requestsInWindow = await redis.zcount(key, now - windowMs, now)

    if (requestsInWindow >= requests) {
      return c.json({ error: 'Rate limit exceeded' }, 429)
    }

    // Add current request
    await redis.zadd(key, now, `${now}-${Math.random()}`)

    // Clean old entries
    await redis.zremrangebyscore(key, 0, now - windowMs)

    // Set expiration
    await redis.expire(key, Math.ceil(windowMs / 1000))

    await next()
  }
}
```

## Redis Caching Middleware

```typescript
// src/middleware/cache.ts
import { MiddlewareHandler } from 'hono'
import { Redis } from 'redis'

const redis = new Redis(process.env.REDIS_URL!)

export const cache = (ttlSeconds: number = 300): MiddlewareHandler => {
  return async (c, next) => {
    const key = `cache:${c.req.method}:${c.req.path}:${JSON.stringify(c.req.queries())}`

    // Try to get from cache
    const cached = await redis.get(key)
    if (cached) {
      return c.json(JSON.parse(cached))
    }

    await next()

    // Cache the response if successful
    if (c.res.status === 200) {
      const body = await c.res.clone().json()
      await redis.setex(key, ttlSeconds, JSON.stringify(body))
    }
  }
}
```

## Backend Proxy Mode (Go Backend)

### Proxy Service
```typescript
// src/services/backendProxy.ts
export class BackendProxy {
  private backendUrl: string

  constructor(backendUrl: string) {
    this.backendUrl = backendUrl
  }

  async proxyRequest(c: Context, path: string, method: string = 'GET') {
    const url = `${this.backendUrl}${path}`

    // Forward headers (including Authorization)
    const headers = new Headers()
    for (const [key, value] of c.req.raw.headers) {
      if (key.toLowerCase() !== 'host') {
        headers.set(key, value)
      }
    }

    // Add user context from JWT
    const userId = c.get('userId')
    if (userId) {
      headers.set('X-User-ID', userId)
      headers.set('X-User-Role', c.get('userRole') || 'user')
    }

    let body: string | undefined
    if (method !== 'GET' && method !== 'HEAD') {
      body = await c.req.text()
    }

    const response = await fetch(url, {
      method,
      headers,
      body,
    })

    const responseBody = await response.text()

    return new Response(responseBody, {
      status: response.status,
      headers: response.headers,
    })
  }
}
```

### Routes for Go Backend
```typescript
// src/routes/api.ts
import { Hono } from 'hono'
import { BackendProxy } from '../services/backendProxy'

const api = new Hono()
const proxy = new BackendProxy(process.env.BACKEND_URL!)

// Proxy authenticated routes to Go backend
api.use('/profile', jwtAuth(), extractUser)
api.all('/profile', async (c) => {
  return proxy.proxyRequest(c, '/api/v1/profile', c.req.method)
})

api.use('/tasks', jwtAuth(), extractUser)
api.all('/tasks', async (c) => {
  return proxy.proxyRequest(c, '/api/v1/tasks', c.req.method)
})

api.use('/tasks/:id', jwtAuth(), extractUser)
api.all('/tasks/:id', async (c) => {
  return proxy.proxyRequest(c, `/api/v1/tasks/${c.req.param('id')}`, c.req.method)
})

export default api
```

## Supabase Mode

### Supabase Service
```typescript
// src/services/supabaseService.ts
import { createClient, SupabaseClient } from '@supabase/supabase-js'

export class SupabaseService {
  private supabase: SupabaseClient

  constructor() {
    this.supabase = createClient(
      process.env.SUPABASE_URL!,
      process.env.SUPABASE_SERVICE_ROLE_KEY!
    )
  }

  // Auth methods
  async signIn(email: string, password: string) {
    const { data, error } = await this.supabase.auth.signInWithPassword({
      email,
      password,
    })
    if (error) throw error
    return data
  }

  async getUserProfile(userId: string) {
    const { data, error } = await this.supabase
      .from('profiles')
      .select('*')
      .eq('id', userId)
      .single()

    if (error) throw error
    return data
  }

  async updateUserProfile(userId: string, updates: any) {
    const { data, error } = await this.supabase
      .from('profiles')
      .update(updates)
      .eq('id', userId)
      .select()
      .single()

    if (error) throw error
    return data
  }

  // Task methods
  async getTasks(userId: string) {
    const { data, error } = await this.supabase
      .from('tasks')
      .select('*')
      .eq('user_id', userId)
      .order('created_at', { ascending: false })

    if (error) throw error
    return data
  }

  async createTask(userId: string, task: any) {
    const { data, error } = await this.supabase
      .from('tasks')
      .insert({ ...task, user_id: userId })
      .select()
      .single()

    if (error) throw error
    return data
  }
}
```

### Routes for Supabase
```typescript
// src/routes/auth.ts
import { Hono } from 'hono'
import { z } from 'zod'
import { SupabaseService } from '../services/supabaseService'

const auth = new Hono()
const supabase = new SupabaseService()

const loginSchema = z.object({
  email: z.string().email(),
  password: z.string().min(6),
})

auth.post('/login', async (c) => {
  try {
    const body = await c.req.json()
    const { email, password } = loginSchema.parse(body)

    const authData = await supabase.signIn(email, password)

    return c.json({
      access_token: authData.session?.access_token,
      refresh_token: authData.session?.refresh_token,
      user: {
        id: authData.user?.id,
        email: authData.user?.email,
      }
    })
  } catch (error) {
    return c.json({ error: 'Invalid credentials' }, 401)
  }
})

auth.post('/refresh', async (c) => {
  // Implement refresh token logic
  return c.json({ error: 'Not implemented' }, 501)
})

export default auth
```

```typescript
// src/routes/users.ts
import { Hono } from 'hono'
import { SupabaseService } from '../services/supabaseService'
import { jwtAuth, extractUser } from '../middleware/auth'

const users = new Hono()
const supabase = new SupabaseService()

users.use('*', jwtAuth(), extractUser)

users.get('/profile', async (c) => {
  try {
    const userId = c.get('userId')
    const profile = await supabase.getUserProfile(userId)

    return c.json(profile)
  } catch (error) {
    return c.json({ error: 'Profile not found' }, 404)
  }
})

users.put('/profile', async (c) => {
  try {
    const userId = c.get('userId')
    const updates = await c.req.json()

    const profile = await supabase.updateUserProfile(userId, updates)

    return c.json(profile)
  } catch (error) {
    return c.json({ error: 'Update failed' }, 400)
  }
})

export default users
```

## Main Application Assembly

```typescript
// src/index.ts
import { Hono } from 'hono'
import { cors } from 'hono/cors'
import { logger } from 'hono/logger'
import authRoutes from './routes/auth'
import userRoutes from './routes/users'
import taskRoutes from './routes/tasks'
import { rateLimit } from './middleware/rateLimit'
import { cache } from './middleware/cache'

const app = new Hono()

// Global middleware
app.use('*', cors({
  origin: process.env.CORS_ORIGIN || '*',
  allowMethods: ['GET', 'POST', 'PUT', 'DELETE'],
  allowHeaders: ['Content-Type', 'Authorization'],
}))

app.use('*', logger())
app.use('*', rateLimit(
  parseInt(process.env.RATE_LIMIT_REQUESTS || '100'),
  parseInt(process.env.RATE_LIMIT_WINDOW || '900000')
))

// Health check (no rate limit)
app.get('/health', (c) => c.json({
  status: 'ok',
  timestamp: new Date().toISOString(),
  mode: process.env.MODE
}))

// API routes
app.route('/api/v1/auth', authRoutes)
app.route('/api/v1/users', userRoutes)
app.route('/api/v1/tasks', taskRoutes)

// Cached routes (for Supabase mode)
if (process.env.MODE === 'supabase') {
  app.use('/api/v1/users/profile', cache(3600)) // 1 hour
  app.use('/api/v1/tasks', cache(300)) // 5 minutes
}

// Error handling
app.onError((err, c) => {
  console.error(`${err}`)
  return c.json({ error: 'Internal server error' }, 500)
})

app.notFound((c) => {
  return c.json({ error: 'Not found' }, 404)
})

export default app
```

## Server Startup

```typescript
// src/server.ts
import app from './index'

const port = process.env.PORT || 3000

console.log(`🚀 Server starting on port ${port}`)
console.log(`📝 Mode: ${process.env.MODE}`)
console.log(`🔗 Backend: ${process.env.BACKEND_URL || 'N/A'}`)
console.log(`🗄️  Supabase: ${process.env.SUPABASE_URL || 'N/A'}`)

export default {
  port,
  fetch: app.fetch,
}
```

## Package.json Scripts

```json
{
  "scripts": {
    "dev": "tsx watch src/server.ts",
    "build": "tsc",
    "start": "node dist/server.js",
    "test": "jest",
    "lint": "eslint src/**/*.ts",
    "type-check": "tsc --noEmit"
  }
}
```

## Docker Deployment

```dockerfile
# Dockerfile
FROM node:18-alpine

WORKDIR /app

COPY package*.json ./
RUN npm ci --only=production

COPY . .

EXPOSE 3000

CMD ["npm", "start"]
```

```yaml
# docker-compose.yml
version: '3.8'
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  hono-gateway:
    build: .
    ports:
      - "3000:3000"
    environment:
      MODE: go_backend
      BACKEND_URL: http://host.docker.internal:8080
      REDIS_URL: redis://redis:6379
      JWT_SECRET: your-secret-key
    depends_on:
      - redis
```

## Testing

```typescript
// src/__tests__/auth.test.ts
import { testClient } from 'hono/testing'
import app from '../index'

describe('Auth API', () => {
  const client = testClient(app)

  test('health check', async () => {
    const res = await client.health.$get()
    expect(res.status).toBe(200)
    const data = await res.json()
    expect(data.status).toBe('ok')
  })

  test('login with valid credentials', async () => {
    const res = await client.api.v1.auth.login.$post({
      json: {
        email: 'test@example.com',
        password: 'password123'
      }
    })
    expect(res.status).toBe(200)
  })
})
```

## Schema Automation

1. **Supabase Migrations**: Keep `assets/migrations` for Supabase schema and automate `supabase db push` / `supabase migrate` via `scripts/apply-migrations.ts`.
2. **Go Backend Compatibility**: Provide a Postgres SQL folder and run migrations with `node ./scripts/migrate.js --mode=go_backend` that delegates to `golang-migrate` or `psql`.
3. **Mode-aware Bootstrapper**: `scripts/init-schema.ts` inspects `process.env.MODE` to run the correct migration set before the server starts.
4. **Schema Documentation**: Auto-generate schema summaries using `schemats` or `sqlc` to help CMS/CRM teams map collections and tables.
5. **Rollback Support**: Keep rollback scripts versioned and runnable via `yarn rollback --version=20251201`.

## Monitoring Hooks

1. **Tracing**: Wrap routes in OTLP spans (`@opentelemetry/api`) and send traces to Jaeger/Honeycomb for both Supabase and Go backend flows.
2. **Metrics Exporter**: Expose `/metrics` via Prometheus middleware; emit counters for proxy latency, cache hits, Redis buffer size, and upstream availability.
3. **Session Monitoring**: Track Redis session buffer depth using Redis keyspace notifications or periodic `INFO` snapshots and surface as custom metrics.
4. **Error Hooks**: Report errors to Sentry/Honeycomb with request IDs, user roles, and trace correlation.
5. **Deploy Guards**: Health checks monitor connectivity to Go backend or Supabase and Redis; tie them into orchestrator readiness/liveness probes to avoid bad deployments.

## Best Practices

1. **Environment Variables**: Never commit secrets, use `.env.example`
2. **Error Handling**: Centralized error responses, don't leak internal errors
3. **Validation**: Use Zod schemas for all input validation
4. **Logging**: Structured logging with request IDs
5. **Caching**: Appropriate TTL values, cache invalidation on mutations
6. **Security**: HTTPS only, secure headers, input sanitization
7. **Performance**: Connection pooling, request timeouts, circuit breakers
8. **Monitoring**: Health checks, metrics, error tracking
