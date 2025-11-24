# IAM Service Directory Structure

```
iam-service/
├── src/
│   ├── index.ts                    # Main Hono application
│   ├── server.ts                   # Server startup configuration
│   ├── middleware/
│   │   ├── auth.ts                 # JWT authentication middleware
│   │   ├── rateLimit.ts            # Rate limiting middleware
│   │   ├── security.ts             # Security headers middleware
│   │   ├── cors.ts                 # CORS configuration
│   │   └── logging.ts              # Security logging middleware
│   ├── routes/
│   │   ├── auth.ts                 # Authentication routes (login/logout)
│   │   ├── oauth.ts                # OAuth2/OIDC routes
│   │   ├── sessions.ts             # Session management routes
│   │   ├── users.ts                # User profile routes
│   │   └── admin.ts                # Administrative routes
│   ├── services/
│   │   ├── redisService.ts         # Redis operations
│   │   ├── jwtService.ts           # JWT token management
│   │   ├── casdoorService.ts       # Casdoor integration
│   │   ├── oauth2Service.ts        # OAuth2 provider integration
│   │   ├── encryptionService.ts    # Data encryption utilities
│   │   ├── auditService.ts         # Audit logging service
│   │   └── userService.ts          # User management service
│   ├── types/
│   │   ├── auth.ts                 # Authentication types
│   │   ├── user.ts                 # User data types
│   │   ├── session.ts              # Session types
│   │   ├── oauth.ts                # OAuth2 types
│   │   └── common.ts               # Common types and interfaces
│   ├── utils/
│   │   ├── crypto.ts               # Cryptographic utilities
│   │   ├── validation.ts           # Input validation utilities
│   │   ├── errors.ts               # Error handling utilities
│   │   └── helpers.ts              # General helper functions
│   └── __tests__/
│       ├── middleware.test.ts
│       ├── services.test.ts
│       ├── routes.test.ts
│       └── integration.test.ts
├── config/
│   ├── development.ts              # Development configuration
│   ├── production.ts               # Production configuration
│   ├── test.ts                     # Test configuration
│   └── index.ts                    # Configuration loader
├── assets/
│   ├── keys/                       # Cryptographic keys (gitignored)
│   │   ├── jwt-access.pem
│   │   ├── jwt-refresh.pem
│   │   └── encryption.key
│   └── templates/                  # Email/HTML templates
│       ├── email-verification.html
│       ├── password-reset.html
│       └── welcome.html
├── scripts/
│   ├── build.sh                    # Build script
│   ├── deploy.sh                   # Deployment script
│   ├── keygen.sh                  # Key generation script
│   ├── migrate.sh                  # Data migration script
│   └── backup.sh                   # Backup script
├── Dockerfile                      # Docker build configuration
├── docker-compose.yml              # Local development setup
├── docker-compose.test.yml         # Testing environment
├── package.json                    # Node.js dependencies
├── package-lock.json               # Dependency lock file
├── tsconfig.json                   # TypeScript configuration
├── jest.config.js                  # Jest testing configuration
├── eslint.config.js                # ESLint configuration
├── .env.example                    # Environment variables template
├── .env.test                       # Test environment variables
├── .gitignore                      # Git ignore patterns
├── README.md                       # Project documentation
├── SECURITY.md                     # Security documentation
├── PRIVACY.md                      # Privacy/GDPR documentation
└── .dockerignore                   # Docker ignore patterns
```

## Key Architecture Notes

### Layer Responsibilities

1. **src/index.ts** - Main application setup, global middleware, route mounting
2. **src/server.ts** - Server configuration, startup logic
3. **middleware/** - Hono middleware for cross-cutting concerns
   - **auth.ts** - JWT validation, user context extraction
   - **rateLimit.ts** - Request throttling, brute force protection
   - **security.ts** - OWASP security headers, XSS protection
   - **cors.ts** - Cross-origin resource sharing configuration
   - **logging.ts** - Structured security event logging
4. **routes/** - HTTP route handlers
   - **auth.ts** - Login, logout, registration, password reset
   - **oauth.ts** - OAuth2/OIDC flows, provider integrations
   - **sessions.ts** - Session listing, revocation, management
   - **users.ts** - Profile management, account settings
   - **admin.ts** - Administrative functions (user management, audit)
5. **services/** - Business logic and external integrations
   - **redisService.ts** - Session storage, caching, rate limiting
   - **jwtService.ts** - Token generation, validation, refresh
   - **casdoorService.ts** - Casdoor OIDC integration
   - **oauth2Service.ts** - Generic OAuth2 provider support
   - **encryptionService.ts** - Data encryption/decryption
   - **auditService.ts** - Security event logging
   - **userService.ts** - User CRUD operations
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

### Security-First Architecture

#### Data Protection
```
Input Data → Validation → Sanitization → Encryption → Storage
                            ↓
                     Audit Logging (Non-PII)
```

#### Token Flow
```
Login Request → Validation → Multi-Factor → Token Generation → Secure Storage
                                                         ↓
                                                Encrypted Session
```

#### Session Management
```
User Authentication → Session Creation → Encrypted Storage → Token Issuance
       ↓                        ↓                ↓              ↓
   Rate Limiting        IP/User-Agent       AES-256        JWT + Refresh
   Tracking             Binding             Encryption     Token Rotation
```

### Dual Authentication Modes

#### JWT + Session Mode (Recommended)
```
1. User login → Validate credentials → Create session in Redis
2. Issue JWT access token + HTTP-only refresh cookie
3. Client uses JWT for API calls
4. Refresh token rotation on refresh requests
5. Session validation on sensitive operations
```

#### Pure JWT Mode (Stateless)
```
1. User login → Validate credentials → Issue JWT tokens
2. Client stores tokens securely
3. JWT validation on each request
4. Token refresh with rotation
5. No server-side session storage
```

### External Provider Integration

#### Casdoor OIDC Flow
```
Frontend → IAM Service → Casdoor → User Auth → Callback → Token Exchange
     ↓          ↓           ↓         ↓         ↓          ↓
  Login      Redirect    Auth Page  Success   Code      JWT Tokens
  Request    to Casdoor             Granted   Received  Generated
```

#### OAuth2 Generic Flow
```
Frontend → IAM Service → Provider → User Auth → Callback → Profile Sync
     ↓          ↓           ↓         ↓         ↓          ↓
  Login      Redirect    Auth Page  Success   Code      User Data
  Request    to Google              Granted   Received   Retrieved
```

### Redis Data Architecture

```typescript
// Session storage with encryption
session:{sessionId} = {
  userId: "encrypted-uuid",
  data: "aes-256-gcm-encrypted-json",
  meta: {
    created: timestamp,
    expires: timestamp,
    ipHash: "hashed-ip-address",
    uaHash: "hashed-user-agent"
  }
}

// User session index
user:sessions:{userId} = Set<sessionId>

// Refresh token storage
refresh:{tokenHash} = {
  userId: "uuid",
  sessionId: "sessionId",
  expires: timestamp,
  used: boolean,
  rotated: boolean
}

// Rate limiting
ratelimit:{action}:{identifier} = SortedSet<timestamp, attemptId>

// Audit log (compressed)
audit:{date}:{type} = CompressedJSON[]
```

### Encryption Strategy

#### At Rest Encryption
- **AES-256-GCM**: For session data and sensitive fields
- **Key Rotation**: Monthly key rotation with overlap
- **Secure Key Storage**: Environment variables or KMS

#### In Transit
- **TLS 1.3**: End-to-end encryption
- **Certificate Pinning**: For critical external services
- **HSTS**: HTTP Strict Transport Security

### GDPR Compliance Architecture

#### Data Minimization
```
Collected Data → Processing → Storage → Retention → Deletion
      ↓             ↓           ↓          ↓          ↓
   Essential      Consent    Encrypted   Policy     Immediate
   Only          Required    Storage     Based      Removal
```

#### User Rights Implementation
- **Access**: Profile export functionality
- **Rectification**: Profile update endpoints
- **Erasure**: Account deletion with cascade
- **Portability**: Data export in standard formats
- **Restriction**: Data processing controls
- **Objection**: Marketing preference management

### Audit Logging Strategy

#### Security Events (Non-PII)
```json
{
  "event": "user_login",
  "timestamp": "2024-01-01T12:00:00Z",
  "ip_hash": "hashed-ip",
  "user_agent_hash": "hashed-ua",
  "success": true,
  "method": "password"
}
```

#### Compliance Events
```json
{
  "event": "data_export",
  "timestamp": "2024-01-01T12:00:00Z",
  "request_id": "uuid",
  "data_types": ["profile", "sessions"],
  "format": "json"
}
```

### Docker Multi-Stage Build

```dockerfile
# Build stage
FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build

# Security scan stage
FROM aquasec/trivy:latest AS security-scan
COPY --from=builder /app/dist ./dist
RUN trivy filesystem --no-progress --exit-code 1 ./dist

# Production stage
FROM node:18-alpine AS production
RUN apk add --no-cache dumb-init
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production && npm cache clean --force
COPY --from=builder /app/dist ./dist
USER node
EXPOSE 3001
ENTRYPOINT ["dumb-init", "--"]
CMD ["npm", "start"]
```

### Development Setup

```yaml
# docker-compose.yml
version: '3.8'
services:
  iam-service:
    build:
      context: .
      target: development
    ports:
      - "3001:3001"
    environment:
      - NODE_ENV=development
      - REDIS_URL=redis://redis:6379
    volumes:
      - .:/app
      - /app/node_modules
    depends_on:
      - redis
    command: npm run dev

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --appendonly yes --requirepass development
    volumes:
      - redis_data:/data

volumes:
  redis_data:
```

### Testing Strategy

```typescript
// __tests__/integration/auth-flow.test.ts
describe('Complete Authentication Flow', () => {
  test('successful login creates session and tokens', async () => {
    // Arrange
    const loginData = { identifier: 'user@example.com', password: 'password' }

    // Act
    const response = await request(app)
      .post('/api/v1/iam/auth/login')
      .send(loginData)

    // Assert
    expect(response.status).toBe(200)
    expect(response.body).toHaveProperty('access_token')
    expect(response.body).toHaveProperty('refresh_token')
    expect(response.headers['set-cookie']).toBeDefined()

    // Verify session created in Redis
    const sessionId = extractSessionId(response.headers['set-cookie'][0])
    const session = await redis.getSession(sessionId)
    expect(session).toBeDefined()
    expect(session.userId).toBe('expected-user-id')
  })
})
```

### Monitoring & Observability

#### Health Checks
```typescript
GET /health/detailed
{
  "status": "healthy",
  "version": "1.0.0",
  "uptime": "86400s",
  "services": {
    "redis": "connected",
    "casdoor": "reachable"
  },
  "metrics": {
    "active_sessions": 1250,
    "tokens_issued_last_hour": 450,
    "failed_login_attempts": 12
  }
}
```

#### Metrics Collection
- **Authentication Metrics**: Success/failure rates, login methods
- **Session Metrics**: Active sessions, creation/destruction rates
- **Token Metrics**: Issuance, refresh, revocation rates
- **Security Metrics**: Rate limit hits, suspicious activities
- **Performance Metrics**: Response times, error rates

### Deployment Patterns

#### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: iam-service
spec:
  replicas: 3
  selector:
    matchLabels:
      app: iam-service
  template:
    metadata:
      labels:
        app: iam-service
    spec:
      containers:
      - name: iam-service
        image: iam-service:latest
        ports:
        - containerPort: 3001
        env:
        - name: REDIS_URL
          valueFrom:
            secretKeyRef:
              name: iam-secrets
              key: redis-url
        livenessProbe:
          httpGet:
            path: /health
            port: 3001
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 3001
          initialDelaySeconds: 5
          periodSeconds: 5
```

#### Cloud-Native Security
- **Secrets Management**: External secret storage (Vault, AWS Secrets Manager)
- **Certificate Management**: Automatic TLS certificate renewal
- **Network Policies**: Pod-to-pod communication restrictions
- **Security Contexts**: Non-root container execution
- **Resource Limits**: Memory and CPU constraints

### Backup & Recovery

#### Automated Backup
```bash
#!/bin/bash
# scripts/backup.sh
DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backups/$DATE"

# Redis dump
docker exec redis redis-cli --rdb /tmp/dump.rdb
docker cp redis:/tmp/dump.rdb $BACKUP_DIR/redis.rdb

# Configuration backup
cp .env $BACKUP_DIR/
cp config/production.ts $BACKUP_DIR/

# Encrypt backup
tar czf $BACKUP_DIR.tar.gz $BACKUP_DIR
openssl enc -aes-256-cbc -salt -in $BACKUP_DIR.tar.gz -out $BACKUP_DIR.enc

# Upload to secure storage
aws s3 cp $BACKUP_DIR.enc s3://iam-backups/
```

#### Disaster Recovery
1. **Data Recovery**: Restore Redis from backup
2. **Key Recovery**: Restore encryption keys from secure storage
3. **Configuration**: Deploy with backup configuration
4. **Validation**: Test authentication flows
5. **User Communication**: Notify users of potential disruptions

This architecture provides a secure, scalable, and GDPR-compliant IAM service that can integrate with various authentication providers while maintaining data protection and audit capabilities.
