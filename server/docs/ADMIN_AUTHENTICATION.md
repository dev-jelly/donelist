# Admin Authentication System

## Overview

The admin authentication system provides role-based access control (RBAC) with comprehensive audit logging for administrative operations. It supports three role levels: `user`, `admin`, and `superadmin`.

## Architecture

### Components

1. **User Roles** (`internal/user/repository.go`)
   - `RoleUser`: Standard user with no administrative privileges
   - `RoleAdmin`: Administrative user with access to admin endpoints
   - `RoleSuperAdmin`: Super administrator with access to all admin functions

2. **JWT Claims Enhancement** (`internal/auth/jwt.go`)
   - Added `role` field to JWT claims for authorization
   - Role is embedded in both access and refresh tokens
   - Allows for efficient authorization without database lookups

3. **Admin Middleware** (`internal/api/middleware/admin.go`)
   - `RequireAdmin`: Validates admin or superadmin role (with DB lookup)
   - `RequireSuperAdmin`: Validates superadmin role only (with DB lookup)
   - `RequireAdminFromToken`: Fast role check from JWT token (no DB lookup)

4. **Audit Logging** (`internal/admin/audit.go`)
   - Comprehensive logging of all admin actions
   - Tracks IP address, user agent, request details, and response status
   - Supports querying by admin, target user, action, date range, and resource

5. **Admin Service** (`internal/admin/service.go`)
   - High-level admin operations with built-in audit logging
   - User role management
   - User deletion
   - Audit log retrieval

## Database Schema

### Users Table Enhancement
```sql
ALTER TABLE users ADD COLUMN role VARCHAR(20) DEFAULT 'user'
    CHECK (role IN ('user', 'admin', 'superadmin'));
```

### Admin Audit Log Table
```sql
CREATE TABLE admin_audit_log (
    id UUID PRIMARY KEY,
    admin_id UUID NOT NULL REFERENCES users(id),
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_id UUID,
    target_user_id UUID REFERENCES users(id),
    ip_address INET,
    user_agent TEXT,
    request_path VARCHAR(255),
    request_method VARCHAR(10),
    request_body JSONB,
    response_status INTEGER,
    metadata JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
```

### Admin Permissions Table (Future Enhancement)
```sql
CREATE TABLE admin_permissions (
    id UUID PRIMARY KEY,
    admin_id UUID NOT NULL REFERENCES users(id),
    permission VARCHAR(100) NOT NULL,
    granted_by UUID REFERENCES users(id),
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE,
    UNIQUE(admin_id, permission)
);
```

## Usage

### 1. Protecting Admin Endpoints

#### Option A: With Database Lookup (Most Secure)
```go
// Checks role from database on every request
// Use for sensitive operations or when role changes must be immediate
router.POST("/admin/users/:id/role",
    middleware.AuthMiddleware(jwtManager, logger),
    middleware.RequireAdmin(userRepo, logger),
    adminHandler.UpdateUserRole,
)
```

#### Option B: Token-Based (High Performance)
```go
// Checks role from JWT token
// Use for high-throughput endpoints where slight delay in role changes is acceptable
router.GET("/admin/dashboard",
    middleware.AuthMiddleware(jwtManager, logger),
    middleware.RequireAdminFromToken(logger),
    adminHandler.GetDashboard,
)
```

#### Option C: Superadmin Only
```go
// Only superadmin can access
router.POST("/admin/users/:id/delete",
    middleware.AuthMiddleware(jwtManager, logger),
    middleware.RequireSuperAdmin(userRepo, logger),
    adminHandler.DeleteUser,
)
```

### 2. Adding Audit Logging to Admin Endpoints

```go
// Add audit middleware after auth and role check
adminRoutes := router.Group("/admin")
adminRoutes.Use(middleware.AuthMiddleware(jwtManager, logger))
adminRoutes.Use(middleware.RequireAdmin(userRepo, logger))
adminRoutes.Use(middleware.AdminAuditMiddleware(auditRepo, logger))

adminRoutes.POST("/users/:id/role", adminHandler.UpdateUserRole)
adminRoutes.DELETE("/users/:id", adminHandler.DeleteUser)
```

### 3. Manual Audit Logging in Services

```go
func (h *AdminHandler) UpdateUserRole(c *gin.Context) {
    adminID, _ := middleware.GetUserID(c)
    targetUserID, _ := uuid.Parse(c.Param("id"))

    // Store target_user_id in context for audit middleware
    c.Set("target_user_id", targetUserID)

    // Your business logic here...

    // Manual audit logging if needed
    err := h.auditRepo.Log(c.Request.Context(), admin.AuditLogInput{
        AdminID:      adminID,
        Action:       "custom_action",
        ResourceType: "user",
        ResourceID:   &targetUserID,
        Metadata: map[string]interface{}{
            "custom_field": "value",
        },
    })
}
```

### 4. Checking User Roles in Code

```go
// Check if user has admin privileges
if user.IsAdmin() {
    // Admin or superadmin
}

// Check if user is superadmin specifically
if user.IsSuperAdmin() {
    // Only superadmin
}
```

### 5. Querying Audit Logs

```go
// Get all logs for a specific admin
logs, err := auditRepo.GetByAdminID(ctx, adminID, limit, offset)

// Get logs affecting a specific user
logs, err := auditRepo.GetByTargetUserID(ctx, targetUserID, limit, offset)

// Get logs for a specific action
logs, err := auditRepo.GetByAction(ctx, "update_role", limit, offset)

// Get logs within date range
logs, err := auditRepo.GetByDateRange(ctx, startDate, endDate, limit, offset)

// Get specific log by ID
log, err := auditRepo.GetByID(ctx, logID)

// Count logs
count, err := auditRepo.CountByAdminID(ctx, adminID)
```

## Security Considerations

### 1. Role Propagation Delay
- **Token-based checks** (`RequireAdminFromToken`): Role changes take effect after token refresh (up to token expiry time)
- **Database checks** (`RequireAdmin`, `RequireSuperAdmin`): Role changes take effect immediately
- **Recommendation**: Use database checks for critical operations, token checks for read-only operations

### 2. Audit Log Security
- All admin actions are logged with IP address, user agent, and request details
- Audit logs are immutable (no update/delete operations)
- Request bodies are logged (limited to 10KB to prevent abuse)
- Sensitive data should be sanitized before being included in audit logs

### 3. Rate Limiting
Admin endpoints should have additional rate limiting:
```go
adminRoutes.Use(middleware.RateLimitMiddleware(redis, "admin", 100, time.Hour))
```

### 4. Monitoring and Alerting
Set up alerts for:
- Multiple failed admin access attempts
- Bulk user deletions
- Role escalations to superadmin
- Admin actions from unusual IP addresses

### 5. Session Management
- Admin users should have stricter session timeout policies
- Consider requiring re-authentication for sensitive operations
- Implement anomaly detection for admin sessions

## API Key Authentication for Admin Operations

Admin operations can also be performed via API keys with proper scopes:

```go
// Require admin API key
router.POST("/admin/users",
    middleware.APIKeyMiddleware(apiKeyService, logger),
    middleware.RequireScope(apikey.ScopeAdminWrite),
    adminHandler.CreateUser,
)
```

## Testing

### Unit Tests
```bash
go test ./internal/api/middleware -v -run TestRequireAdmin
go test ./internal/api/middleware -v -run TestRequireSuperAdmin
```

### Integration Tests
```bash
go test ./internal/admin -v -run TestAdminAuditIntegration
```

## Migration

To add admin roles to existing deployment:

1. Run migration:
```bash
migrate -path ./migrations -database "postgresql://..." up
```

2. Promote first admin user:
```sql
UPDATE users SET role = 'superadmin' WHERE email = 'your-admin@example.com';
```

3. Update JWT token generation in your auth service to include role

4. Deploy new code with admin middleware

5. Test admin endpoints

## Future Enhancements

1. **Fine-grained Permissions**: Implement `admin_permissions` table for granular access control
2. **Admin Activity Dashboard**: Real-time monitoring of admin actions
3. **Two-Factor Authentication**: Require 2FA for admin accounts
4. **Admin Session Recording**: Record all admin sessions for compliance
5. **Automated Anomaly Detection**: ML-based detection of unusual admin behavior
6. **Time-limited Admin Access**: Temporary admin privileges with automatic expiration
7. **Admin Role Delegation**: Allow admins to delegate specific permissions temporarily

## Example: Complete Admin Endpoint

```go
func SetupAdminRoutes(router *gin.Engine, deps *Dependencies) {
    adminGroup := router.Group("/api/v1/admin")

    // Authentication required for all admin routes
    adminGroup.Use(middleware.AuthMiddleware(deps.JWTManager, deps.Logger))

    // Admin role required
    adminGroup.Use(middleware.RequireAdmin(deps.UserRepo, deps.Logger))

    // Audit all admin actions
    adminGroup.Use(middleware.AdminAuditMiddleware(deps.AuditRepo, deps.Logger))

    // Rate limit admin endpoints
    adminGroup.Use(middleware.RateLimitMiddleware(deps.Redis, "admin", 100, time.Hour))

    // User management
    adminGroup.GET("/users", deps.AdminHandler.ListUsers)
    adminGroup.GET("/users/:id", deps.AdminHandler.GetUser)
    adminGroup.POST("/users/:id/role", deps.AdminHandler.UpdateUserRole)

    // Superadmin only endpoints
    superadminGroup := adminGroup.Group("")
    superadminGroup.Use(middleware.RequireSuperAdmin(deps.UserRepo, deps.Logger))
    superadminGroup.DELETE("/users/:id", deps.AdminHandler.DeleteUser)
    superadminGroup.POST("/admins", deps.AdminHandler.CreateAdmin)

    // Audit logs (admin can view, superadmin can view all)
    adminGroup.GET("/audit-logs", deps.AdminHandler.GetAuditLogs)
}
```

## Troubleshooting

### Admin Access Denied After Role Change
- **Cause**: Token contains old role
- **Solution**: User must refresh token or wait for token expiry

### Audit Logs Not Being Created
- **Check**: AdminAuditMiddleware is applied after RequireAdmin middleware
- **Check**: Database has admin_audit_log table
- **Check**: Context contains "admin_user" key (set by RequireAdmin)

### Performance Issues with Admin Middleware
- **Solution**: Use `RequireAdminFromToken` for high-traffic endpoints
- **Solution**: Add caching layer for role lookups
- **Solution**: Use connection pooling for database

## Compliance

This admin authentication system supports:
- **SOC 2**: Comprehensive audit logging and access controls
- **GDPR**: Audit trails for data access and modifications
- **HIPAA**: Role-based access control and audit logging
- **PCI DSS**: Administrative access controls and logging

All admin actions are logged with sufficient detail to meet most compliance requirements.
