# IAM Service Guide: Authentication Gateway 101

## Prerequisites
- Node.js 18+
- Redis 7+
- Casdoor instance (optional)
- OAuth2 provider accounts (optional)

## Project Setup

### 1. Initialize Project
```bash
mkdir iam-service
cd iam-service
npm init -y
```

### 2. Install Dependencies
```bash
npm install hono @hono/jwt @hono/cors
npm install redis @casdoor/node-sdk oauth2-client
npm install zod bcrypt crypto
npm install -D @types/node typescript tsx jest @types/jest
```

### 3. Environment Configuration
Create `.env` file:
```env
PORT=3001
NODE_ENV=production
REDIS_URL=redis://localhost:6379

# JWT Configuration
JWT_ACCESS_SECRET=your-super-secure-access-secret-key-32-chars
JWT_REFRESH_SECRET=your-super-secure-refresh-secret-key-32-chars
JWT_ACCESS_EXPIRE=900
JWT_REFRESH_EXPIRE=604800

# Session Security
SESSION_ENCRYPTION_KEY=your-32-character-encryption-key-here

# Casdoor Integration
CASDOOR_ENABLED=true
CASDOOR_ENDPOINT=https://your-casdoor-instance.com
CASDOOR_CLIENT_ID=your-casdoor-client-id
CASDOOR_CLIENT_SECRET=your-casdoor-client-secret
CASDOOR_CERTIFICATE=your-casdoor-certificate
CASDOOR_ORG_NAME=your-organization-name

# OAuth2 Providers
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-client-secret
GITHUB_CLIENT_ID=your-github-client-id
GITHUB_CLIENT_SECRET=your-github-client-secret

# Security
CORS_ORIGINS=http://localhost:3000,https://yourapp.com
RATE_LIMIT_LOGIN=5
RATE_LIMIT_WINDOW=900000
```

## Core Application Structure

### Main Application
```typescript
// src/index.ts
import { Hono } from 'hono'
import { cors } from 'hono/cors'
import { logger } from 'hono/logger'
import { RedisService } from './services/redisService'
import { JWTService } from './services/jwtService'
import authRoutes from './routes/auth'
import oauthRoutes from './routes/oauth'
import sessionRoutes from './routes/sessions'
import userRoutes from './routes/users'

const app = new Hono()

// Initialize services
const redis = new RedisService(process.env.REDIS_URL!)
const jwt = new JWTService(
  process.env.JWT_ACCESS_SECRET!,
  process.env.JWT_REFRESH_SECRET!
)

// Global middleware
app.use('*', cors({
  origin: process.env.CORS_ORIGINS?.split(',') || ['*'],
  credentials: true,
  allowMethods: ['GET', 'POST', 'PUT', 'DELETE'],
  allowHeaders: ['Content-Type', 'Authorization', 'X-Requested-With'],
}))

app.use('*', logger())

// Health check
app.get('/health', async (c) => {
  const health = await redis.healthCheck()
  return c.json({
    status: health ? 'healthy' : 'unhealthy',
    timestamp: new Date().toISOString(),
    services: {
      redis: health ? 'connected' : 'disconnected'
    }
  })
})

// Mount routes
app.route('/api/v1/iam/auth', authRoutes)
app.route('/api/v1/iam/oauth', oauthRoutes)
app.route('/api/v1/iam/sessions', sessionRoutes)
app.route('/api/v1/iam/users', userRoutes)

// Error handling
app.onError((err, c) => {
  console.error('Unhandled error:', err)
  return c.json({
    error: 'Internal server error',
    requestId: c.get('requestId')
  }, 500)
})

export { app, redis, jwt }
```

## Core Services

### Redis Service
```typescript
// src/services/redisService.ts
import { Redis } from 'redis'

export interface SessionData {
  userId: string
  encryptedData: string
  expires: number
  ip?: string
  userAgent?: string
}

export class RedisService {
  private client: Redis

  constructor(url: string) {
    this.client = new Redis(url)
  }

  async createSession(sessionId: string, data: SessionData): Promise<void> {
    const key = `session:${sessionId}`
    await this.client.hset(key, data)
    await this.client.expire(key, Math.floor((data.expires - Date.now()) / 1000))
  }

  async getSession(sessionId: string): Promise<SessionData | null> {
    const key = `session:${sessionId}`
    const data = await this.client.hgetall(key)
    return Object.keys(data).length > 0 ? data as any : null
  }

  async destroySession(sessionId: string): Promise<void> {
    const key = `session:${sessionId}`
    await this.client.del(key)
  }

  async getUserSessions(userId: string): Promise<string[]> {
    const key = `user:sessions:${userId}`
    return await this.client.smembers(key)
  }

  async storeRefreshToken(tokenHash: string, data: {
    userId: string
    sessionId: string
    expires: number
  }): Promise<void> {
    const key = `refresh:${tokenHash}`
    await this.client.hset(key, data)
    await this.client.expire(key, Math.floor((data.expires - Date.now()) / 1000))
  }

  async getRefreshToken(tokenHash: string): Promise<any> {
    const key = `refresh:${tokenHash}`
    return await this.client.hgetall(key)
  }

  async markRefreshTokenUsed(tokenHash: string): Promise<void> {
    const key = `refresh:${tokenHash}`
    await this.client.hset(key, 'used', 'true')
    await this.client.expire(key, 60) // Keep for 1 minute then delete
  }

  async healthCheck(): Promise<boolean> {
    try {
      await this.client.ping()
      return true
    } catch {
      return false
    }
  }

  async recordLoginAttempt(identifier: string, success: boolean): Promise<void> {
    const key = `ratelimit:login:${identifier}`
    const now = Date.now()

    await this.client.zadd(key, now, `${now}-${success}`)
    await this.client.zremrangebyscore(key, 0, now - 900000) // Keep 15 minutes
    await this.client.expire(key, 900) // Expire after 15 minutes
  }

  async getLoginAttempts(identifier: string, windowMs: number = 900000): Promise<number> {
    const key = `ratelimit:login:${identifier}`
    const now = Date.now()
    return await this.client.zcount(key, now - windowMs, now)
  }
}
```

### JWT Service
```typescript
// src/services/jwtService.ts
import { sign, verify } from 'hono/jwt'

export interface JWTPayload {
  sub: string      // User ID
  iat: number      // Issued at
  exp: number      // Expires at
  jti?: string     // JWT ID
  role?: string    // User role
}

export class JWTService {
  constructor(
    private accessSecret: string,
    private refreshSecret: string
  ) {}

  async generateAccessToken(payload: Omit<JWTPayload, 'iat' | 'exp'>): Promise<string> {
    const tokenPayload = {
      ...payload,
      iat: Math.floor(Date.now() / 1000),
      exp: Math.floor(Date.now() / 1000) + parseInt(process.env.JWT_ACCESS_EXPIRE || '900'),
    }

    return await sign(tokenPayload, this.accessSecret)
  }

  async generateRefreshToken(payload: Omit<JWTPayload, 'iat' | 'exp'>): Promise<string> {
    const tokenPayload = {
      ...payload,
      iat: Math.floor(Date.now() / 1000),
      exp: Math.floor(Date.now() / 1000) + parseInt(process.env.JWT_REFRESH_EXPIRE || '604800'),
    }

    return await sign(tokenPayload, this.refreshSecret)
  }

  async verifyAccessToken(token: string): Promise<JWTPayload> {
    return await verify(token, this.accessSecret) as JWTPayload
  }

  async verifyRefreshToken(token: string): Promise<JWTPayload> {
    return await verify(token, this.refreshSecret) as JWTPayload
  }

  generateSecureToken(length: number = 32): string {
    return require('crypto').randomBytes(length).toString('hex')
  }
}
```

## Authentication Routes

### Login/Registration
```typescript
// src/routes/auth.ts
import { Hono } from 'hono'
import { z } from 'zod'
import { RedisService, JWTService } from '../services'

const auth = new Hono()
const redis = new RedisService(process.env.REDIS_URL!)
const jwt = new JWTService(process.env.JWT_ACCESS_SECRET!, process.env.JWT_REFRESH_SECRET!)

const loginSchema = z.object({
  identifier: z.string().min(1), // email or username
  password: z.string().min(6),
})

const registerSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
  firstName: z.string().min(1),
  lastName: z.string().min(1),
})

auth.post('/login', async (c) => {
  try {
    const body = await c.req.json()
    const { identifier, password } = loginSchema.parse(body)

    // Rate limiting check
    const attempts = await redis.getLoginAttempts(identifier)
    if (attempts >= parseInt(process.env.RATE_LIMIT_LOGIN || '5')) {
      return c.json({ error: 'Too many login attempts' }, 429)
    }

    // Here you would validate credentials against your user store
    // For demo, we'll assume validation passes
    const userId = 'user-123' // From your user validation logic
    const role = 'user'

    // Record login attempt
    await redis.recordLoginAttempt(identifier, true)

    // Generate tokens
    const accessToken = await jwt.generateAccessToken({ sub: userId, role })
    const refreshToken = await jwt.generateRefreshToken({ sub: userId })

    // Create session
    const sessionId = jwt.generateSecureToken()
    await redis.createSession(sessionId, {
      userId,
      encryptedData: JSON.stringify({ role, loginTime: Date.now() }),
      expires: Date.now() + (24 * 60 * 60 * 1000), // 24 hours
      ip: c.req.header('CF-Connecting-IP') || c.req.header('X-Forwarded-For'),
      userAgent: c.req.header('User-Agent'),
    })

    // Store refresh token
    const refreshHash = require('crypto').createHash('sha256').update(refreshToken).digest('hex')
    await redis.storeRefreshToken(refreshHash, {
      userId,
      sessionId,
      expires: Date.now() + (7 * 24 * 60 * 60 * 1000), // 7 days
    })

    // Set secure cookie
    c.cookie('session_id', sessionId, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'strict',
      maxAge: 24 * 60 * 60, // 24 hours
    })

    return c.json({
      access_token: accessToken,
      refresh_token: refreshToken,
      token_type: 'Bearer',
      expires_in: 900,
      user: {
        id: userId,
        role: role,
      }
    })

  } catch (error) {
    if (error instanceof z.ZodError) {
      return c.json({ error: 'Invalid input', details: error.errors }, 400)
    }
    return c.json({ error: 'Authentication failed' }, 401)
  }
})

auth.post('/refresh', async (c) => {
  try {
    const body = await c.req.json()
    const { refresh_token } = body

    if (!refresh_token) {
      return c.json({ error: 'Refresh token required' }, 400)
    }

    // Verify refresh token
    const refreshPayload = await jwt.verifyRefreshToken(refresh_token)

    // Check if token has been used
    const refreshHash = require('crypto').createHash('sha256').update(refresh_token).digest('hex')
    const storedToken = await redis.getRefreshToken(refreshHash)

    if (!storedToken || storedToken.used) {
      return c.json({ error: 'Invalid refresh token' }, 401)
    }

    // Mark token as used (rotation)
    await redis.markRefreshTokenUsed(refreshHash)

    // Generate new tokens
    const newAccessToken = await jwt.generateAccessToken({
      sub: refreshPayload.sub,
      role: refreshPayload.role
    })
    const newRefreshToken = await jwt.generateRefreshToken({
      sub: refreshPayload.sub
    })

    // Store new refresh token
    const newRefreshHash = require('crypto').createHash('sha256').update(newRefreshToken).digest('hex')
    await redis.storeRefreshToken(newRefreshHash, {
      userId: refreshPayload.sub,
      sessionId: storedToken.sessionId,
      expires: Date.now() + (7 * 24 * 60 * 60 * 1000),
    })

    return c.json({
      access_token: newAccessToken,
      refresh_token: newRefreshToken,
      token_type: 'Bearer',
      expires_in: 900,
    })

  } catch (error) {
    return c.json({ error: 'Invalid refresh token' }, 401)
  }
})

auth.post('/logout', async (c) => {
  try {
    const sessionId = c.req.header('Cookie')?.match(/session_id=([^;]+)/)?.[1]

    if (sessionId) {
      await redis.destroySession(sessionId)
    }

    // Clear cookie
    c.cookie('session_id', '', {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'strict',
      maxAge: 0,
    })

    return c.json({ message: 'Logged out successfully' })

  } catch (error) {
    return c.json({ error: 'Logout failed' }, 500)
  }
})

export default auth
```

## OAuth2 Integration

### Casdoor OAuth2
```typescript
// src/services/casdoorService.ts
import Sdk from '@casdoor/node-sdk'

export class CasdoorService {
  private sdk: any

  constructor() {
    this.sdk = Sdk.newClient({
      endpoint: process.env.CASDOOR_ENDPOINT!,
      clientId: process.env.CASDOOR_CLIENT_ID!,
      clientSecret: process.env.CASDOOR_CLIENT_SECRET!,
      certificate: process.env.CASDOOR_CERTIFICATE!,
      orgName: process.env.CASDOOR_ORG_NAME!,
    })
  }

  getAuthUrl(redirectUri: string): string {
    return this.sdk.getAuthUrl(redirectUri)
  }

  async getToken(code: string, redirectUri: string): Promise<any> {
    return await this.sdk.getToken(code, redirectUri)
  }

  async getUserInfo(token: string): Promise<any> {
    return await this.sdk.getUserInfo(token)
  }

  async refreshToken(refreshToken: string): Promise<any> {
    return await this.sdk.refreshToken(refreshToken)
  }
}
```

### OAuth2 Routes
```typescript
// src/routes/oauth.ts
import { Hono } from 'hono'
import { CasdoorService } from '../services/casdoorService'
import { RedisService, JWTService } from '../services'

const oauth = new Hono()
const casdoor = new CasdoorService()
const redis = new RedisService(process.env.REDIS_URL!)
const jwt = new JWTService(process.env.JWT_ACCESS_SECRET!, process.env.JWT_REFRESH_SECRET!)

oauth.get('/casdoor', (c) => {
  const redirectUri = `${c.req.header('origin')}/api/v1/iam/oauth/callback`
  const authUrl = casdoor.getAuthUrl(redirectUri)
  return c.redirect(authUrl)
})

oauth.get('/callback', async (c) => {
  try {
    const code = c.req.query('code')
    const state = c.req.query('state')

    if (!code) {
      return c.json({ error: 'Authorization code missing' }, 400)
    }

    const redirectUri = `${c.req.header('origin')}/api/v1/iam/oauth/callback`
    const tokenData = await casdoor.getToken(code, redirectUri)
    const userInfo = await casdoor.getUserInfo(tokenData.access_token)

    // Create or update user in your system
    const userId = userInfo.id || userInfo.sub
    const role = userInfo.role || 'user'

    // Generate our tokens
    const accessToken = await jwt.generateAccessToken({ sub: userId, role })
    const refreshToken = await jwt.generateRefreshToken({ sub: userId })

    // Create session
    const sessionId = jwt.generateSecureToken()
    await redis.createSession(sessionId, {
      userId,
      encryptedData: JSON.stringify({
        role,
        provider: 'casdoor',
        loginTime: Date.now()
      }),
      expires: Date.now() + (24 * 60 * 60 * 1000),
    })

    // Set cookie and redirect to frontend
    c.cookie('session_id', sessionId, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'strict',
      maxAge: 24 * 60 * 60,
    })

    // Redirect to frontend with tokens
    const frontendUrl = process.env.FRONTEND_URL || 'http://localhost:3000'
    return c.redirect(`${frontendUrl}/auth/callback?access_token=${accessToken}&refresh_token=${refreshToken}`)

  } catch (error) {
    console.error('OAuth callback error:', error)
    return c.json({ error: 'Authentication failed' }, 500)
  }
})

export default oauth
```

## Session Management

```typescript
// src/routes/sessions.ts
import { Hono } from 'hono'
import { RedisService, JWTService } from '../services'
import { jwtAuth } from '../middleware/auth'

const sessions = new Hono()
const redis = new RedisService(process.env.REDIS_URL!)
const jwt = new JWTService(process.env.JWT_ACCESS_SECRET!, process.env.JWT_REFRESH_SECRET!)

sessions.use('*', jwtAuth)

sessions.get('/active', async (c) => {
  try {
    const userId = c.get('userId')
    const sessionIds = await redis.getUserSessions(userId)

    const sessions = []
    for (const sessionId of sessionIds) {
      const session = await redis.getSession(sessionId)
      if (session) {
        sessions.push({
          id: sessionId,
          created: session.created,
          expires: session.expires,
          ip: session.ip,
          userAgent: session.userAgent,
        })
      }
    }

    return c.json({ sessions })

  } catch (error) {
    return c.json({ error: 'Failed to get sessions' }, 500)
  }
})

sessions.delete('/:sessionId', async (c) => {
  try {
    const sessionId = c.req.param('sessionId')
    const userId = c.get('userId')

    // Verify session belongs to user
    const session = await redis.getSession(sessionId)
    if (!session || session.userId !== userId) {
      return c.json({ error: 'Session not found' }, 404)
    }

    await redis.destroySession(sessionId)
    return c.json({ message: 'Session revoked' })

  } catch (error) {
    return c.json({ error: 'Failed to revoke session' }, 500)
  }
})

export default sessions
```

## Security Middleware

```typescript
// src/middleware/auth.ts
import { MiddlewareHandler } from 'hono'
import { JWTService } from '../services/jwtService'

export const jwtAuth = (): MiddlewareHandler => {
  const jwt = new JWTService(process.env.JWT_ACCESS_SECRET!, process.env.JWT_REFRESH_SECRET!)

  return async (c, next) => {
    const authHeader = c.req.header('Authorization')

    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return c.json({ error: 'Authorization header required' }, 401)
    }

    const token = authHeader.substring(7)

    try {
      const payload = await jwt.verifyAccessToken(token)
      c.set('userId', payload.sub)
      c.set('userRole', payload.role)
      await next()
    } catch (error) {
      return c.json({ error: 'Invalid or expired token' }, 401)
    }
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

EXPOSE 3001

CMD ["npm", "start"]
```

```yaml
# docker-compose.yml
version: '3.8'
services:
  iam-service:
    build: .
    ports:
      - "3001:3001"
    environment:
      - REDIS_URL=redis://redis:6379
      - JWT_ACCESS_SECRET=your-access-secret
      - JWT_REFRESH_SECRET=your-refresh-secret
      - SESSION_ENCRYPTION_KEY=your-encryption-key
    depends_on:
      - redis

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
```

## Testing

```typescript
// src/__tests__/auth.test.ts
import { testClient } from 'hono/testing'
import { app } from '../index'

describe('IAM Authentication', () => {
  const client = testClient(app)

  test('health check', async () => {
    const res = await client.health.$get()
    expect(res.status).toBe(200)
  })

  test('login requires valid input', async () => {
    const res = await client.api.v1.iam.auth.login.$post({
      json: { identifier: '', password: '' }
    })
    expect(res.status).toBe(400)
  })

  test('protected route requires auth', async () => {
    const res = await client.api.v1.iam.users.profile.$get()
    expect(res.status).toBe(401)
  })
})
```

## Schema Automation

1. **Database Migrations**: Version all Redis schema (keys/prefixes) and Casdoor/Postgres migrations in `assets/migrations`. Use `node scripts/migrate.ts` to apply them.
2. **Automated Setup**: Provide `scripts/bootstrap.ts` that creates default audit tables, session prefixes, and OAuth provider configs before the service starts.
3. **Redis Schemas**: Keep documentation of key prefixes (`session:{id}`, `refresh:{hash}`) and expose a `scripts/keys.md` to help teams map session lifetime to storage.
4. **Schema Docs**: Generate reference documentation with `schemats` or `sqlc` for Postgres tables used by Casdoor/audit logs.
5. **Rollback & Seed**: Include `scripts/rollback.ts` and `scripts/seed.ts` that can undo migrations and seed initial admin/user entries for testing.

## Monitoring Hooks

1. **Tracing**: Instrument all authentication flows and OAuth callbacks with OpenTelemetry spans to provide end-to-end traceability.
2. **Metrics Exporter**: Expose `/metrics` for Prometheus; include gauges for active sessions, refresh token rotations, rate-limit hits, and Casdoor availability.
3. **Redis Health Hooks**: Monitor Redis latency, replication status, and key evictions; surface alerts for session buffer saturation or failed writes.
4. **Audit Hooks**: Emit security-relevant events (login failures, token refreshes) to log aggregator/Sentry with correlation IDs.
5. **Deployment Probes**: Provide `/health/detailed` that checks Redis and Casdoor, feeding readiness/liveness probes for orchestrators.

## Best Practices

1. **Token Security**: Use strong secrets, implement token rotation
2. **Session Management**: Encrypt session data, implement secure cookies
3. **Rate Limiting**: Protect against brute force attacks
4. **Input Validation**: Validate all inputs, sanitize data
5. **Error Handling**: Don't leak sensitive information in errors
6. **Logging**: Log security events without PII
7. **GDPR Compliance**: Implement data deletion, consent management
8. **Monitoring**: Track authentication metrics and anomalies
