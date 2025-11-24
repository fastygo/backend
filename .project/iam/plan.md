# IAM Service Plan: Authentication & Authorization Gateway

## Overview
Dedicated Identity and Access Management service built with Hono and Redis. Provides secure authentication, session management, and integration with external IAM providers like Casdoor. Acts as a central authority for user identity while maintaining GDPR compliance.

## Core Requirements
- **Security First**: Military-grade encryption, secure token handling
- **Provider Integration**: Support for Casdoor, OAuth2, SAML
- **Session Management**: Secure Redis-backed sessions
- **GDPR Compliance**: Data minimization, user consent, audit trails
- **High Availability**: Stateless design, horizontal scaling
- **Audit Logging**: Complete activity tracking without PII
- **No Offline Mode**: IAM remains online-only for security reasons

## Architecture Overview
```
External IAM (Casdoor) ←→ Hono IAM Gateway ←→ Redis Sessions
                              ↓
                        Frontend Apps
                              ↓
                    Backend Services (Go)
```

## Core Features

### Authentication Methods
- **JWT Tokens**: Access and refresh token pairs
- **Session Cookies**: Secure HTTP-only cookies
- **OAuth2 Flows**: Authorization code, implicit, PKCE
- **SAML Integration**: Enterprise SSO support
- **MFA Support**: TOTP, SMS, email verification

### Session Management
- **Redis Storage**: Encrypted session data
- **Automatic Cleanup**: Expired session removal
- **Session Fixation Protection**: Session ID regeneration
- **Concurrent Session Control**: Configurable limits
- **Device Tracking**: Trusted device management

### Provider Integrations

#### Casdoor Integration
- **OIDC Client**: OpenID Connect flows
- **User Sync**: Automatic user provisioning
- **Role Mapping**: Casdoor roles to internal permissions
- **SCIM Support**: User lifecycle management

#### OAuth2 Providers
- Google, GitHub, Facebook, Microsoft
- Custom OAuth2 server support
- Social login with profile enrichment

## API Endpoints Structure
```
/api/v1/iam/
├── auth/
│   ├── login                 # POST - user login
│   ├── logout                # POST - user logout
│   ├── refresh               # POST - token refresh
│   ├── register              # POST - user registration
│   └── verify                # POST - email/MFA verification
├── oauth/
│   ├── authorize             # GET - OAuth2 authorization
│   ├── callback              # GET - OAuth2 callback
│   ├── {provider}            # GET - initiate provider auth
│   └── token                 # POST - token exchange
├── sessions/
│   ├── active                # GET - list user sessions
│   ├── revoke/{id}           # DELETE - revoke session
│   └── current               # GET - current session info
├── users/
│   ├── profile               # GET - user profile (minimal)
│   ├── update                # PUT - update profile
│   └── delete                # DELETE - account deletion
└── admin/
    ├── users                 # GET - user management
    ├── sessions              # GET - session monitoring
    └── audit                 # GET - audit logs
```

## Technology Stack
```json
{
  "hono": "^4.0.0",
  "hono/jwt": "^1.0.0",
  "hono/cors": "^1.0.0",
  "redis": "^4.6.0",
  "@casdoor/node-sdk": "^1.0.0",
  "oauth2-client": "^1.0.0",
  "zod": "^3.22.0",
  "bcrypt": "^5.1.0",
  "crypto": "node:crypto"
}
```

## Environment Configuration

### Core Configuration
```env
PORT=3001
NODE_ENV=production
REDIS_URL=redis://redis:6379
JWT_ACCESS_SECRET=your-access-secret-key
JWT_REFRESH_SECRET=your-refresh-secret-key
JWT_ACCESS_EXPIRE=900          # 15 minutes
JWT_REFRESH_EXPIRE=604800      # 7 days
SESSION_ENCRYPTION_KEY=32-char-encryption-key
```

### Casdoor Configuration
```env
CASDOOR_ENABLED=true
CASDOOR_ENDPOINT=https://your-casdoor.com
CASDOOR_CLIENT_ID=your-client-id
CASDOOR_CLIENT_SECRET=your-client-secret
CASDOOR_CERTIFICATE=your-certificate
CASDOOR_ORG_NAME=your-organization
```

### OAuth2 Configuration
```env
GOOGLE_CLIENT_ID=your-google-client-id
GOOGLE_CLIENT_SECRET=your-google-secret
GITHUB_CLIENT_ID=your-github-client-id
GITHUB_CLIENT_SECRET=your-github-secret
```

## Security Features

### Token Security
- **JWT Algorithm**: RS256 with key rotation
- **Token Encryption**: AES-256-GCM encryption
- **Secure Storage**: HTTP-only, secure, same-site cookies
- **Token Revocation**: Instant revocation capability
- **Refresh Token Rotation**: One-time use refresh tokens

### Session Security
- **Session Encryption**: AES-256 encryption at rest
- **Secure IDs**: Cryptographically secure session IDs
- **IP Tracking**: Session IP binding options
- **Device Fingerprinting**: Basic device tracking
- **Suspicious Activity**: Anomaly detection

### GDPR Compliance Features
- **Data Minimization**: Only essential user data
- **Consent Management**: Explicit user consent tracking
- **Right to Erasure**: Complete data deletion
- **Data Portability**: User data export
- **Audit Trails**: Non-PII activity logging
- **Retention Policies**: Configurable data retention

## Middleware Stack
1. **Request ID**: Unique tracing identifier
2. **Security Headers**: OWASP security headers
3. **Rate Limiting**: Anti-brute force protection
4. **CORS**: Configured cross-origin access
5. **Logging**: Structured security logging
6. **Input Validation**: Request sanitization
7. **Session Validation**: Active session verification

## Data Storage Strategy

### Redis Data Structures
```javascript
// Session storage
session:{sessionId} = {
  userId: "uuid",
  encryptedData: "encrypted-payload",
  expires: timestamp,
  ip: "hashed-ip",
  userAgent: "hashed-ua"
}

// User sessions index
user:sessions:{userId} = ["sessionId1", "sessionId2"]

// Refresh tokens
refresh:{tokenHash} = {
  userId: "uuid",
  sessionId: "sessionId",
  expires: timestamp,
  used: false
}

// Rate limiting
ratelimit:login:{identifier} = sorted-set-of-attempts
```

### Data Encryption
- **At Rest**: AES-256-GCM encryption
- **In Transit**: TLS 1.3 encryption
- **Key Management**: Key rotation, secure key storage
- **Salt Usage**: Unique salts per user/data

## External Provider Integration

### Casdoor OIDC Flow
```typescript
// 1. Redirect to Casdoor
GET /oauth/casdoor → redirect to Casdoor authorize endpoint

// 2. Casdoor callback
GET /oauth/callback?code=... → exchange code for tokens

// 3. User info from Casdoor
POST /api/oidc/userinfo → get user profile

// 4. Create local session
// Store minimal user data, create JWT tokens
```

### OAuth2 Generic Flow
```typescript
// 1. Initiate OAuth2
GET /oauth/{provider} → redirect to provider

// 2. Provider callback
GET /oauth/callback?code=... → exchange for access token

// 3. Get user profile
GET provider-API/user → fetch user information

// 4. Create account/session
// Map provider data to internal user model
```

## Monitoring & Compliance

### Audit Logging
- **Non-PII Events**: Login, logout, token refresh
- **Security Events**: Failed login, suspicious activity
- **Admin Actions**: User management, configuration changes
- **Compliance Events**: Data export, deletion requests

### Health Checks
```typescript
GET /health/detailed
{
  "status": "healthy",
  "redis": "connected",
  "casdoor": "reachable",
  "uptime": "123456s",
  "active_sessions": 1250
}
```

### Metrics Collection
- **Authentication Success/Failure Rates**
- **Session Creation/Destruction**
- **Token Refresh Patterns**
- **Provider Integration Health**
- **Rate Limiting Events**

## Deployment Architecture

### Production Setup
```yaml
# docker-compose.prod.yml
version: '3.8'
services:
  iam-service:
    image: iam-service:latest
    environment:
      - REDIS_URL=redis://redis:6379
      - CASDOOR_ENDPOINT=https://casdoor.company.com
    secrets:
      - jwt_secrets
      - casdoor_credentials
    deploy:
      replicas: 3
      restart_policy:
        condition: on-failure

  redis:
    image: redis:7-alpine
    command: redis-server --appendonly yes
    volumes:
      - redis_data:/data
    deploy:
      replicas: 1

  casdoor:
    image: casdoor/casdoor:latest
    environment:
      - CASDOOR_DRIVER=postgres
    depends_on:
      - postgres
```

### Scaling Considerations
- **Stateless Design**: Horizontal scaling support
- **Redis Clustering**: Session storage scaling
- **Load Balancing**: Distribute requests across instances
- **Database Sharding**: User data partitioning if needed

## Disaster Recovery
- **Session Backup**: Redis persistence and replication
- **Key Management**: Secure key backup and recovery
- **Audit Log Archiving**: Long-term compliance storage
- **Failover Procedures**: Automatic failover to backup instances

## Compliance Checklist
- [ ] GDPR data processing agreement
- [ ] Data retention policies implemented
- [ ] Right to erasure functionality
- [ ] Audit logging for all user actions
- [ ] Data export capabilities
- [ ] Consent management system
- [ ] Security incident response plan
- [ ] Regular security assessments
