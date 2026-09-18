package admin_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/dev-jelly/donelist/internal/admin"
	"github.com/dev-jelly/donelist/internal/testutil"
	"github.com/dev-jelly/donelist/internal/user"
	"go.uber.org/zap"
)

func TestAdminAuditIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup test database
	db := testutil.SetupTestDB(t)
	defer testutil.TeardownTestDB(t, db)

	ctx := context.Background()
	logger := zap.NewNop()

	// Create repositories
	userRepo := user.NewRepository(db)
	auditRepo := admin.NewAuditRepository(db)
	adminService := admin.NewService(userRepo, auditRepo, logger)

	// Create test admin user
	adminUser, err := userRepo.Create(ctx, user.CreateUserInput{
		Email:        "admin@example.com",
		PasswordHash: "hashed_password",
	})
	require.NoError(t, err)

	// Update admin role
	err = userRepo.UpdateRole(ctx, adminUser.ID, user.RoleAdmin)
	require.NoError(t, err)

	// Create test target user
	targetUser, err := userRepo.Create(ctx, user.CreateUserInput{
		Email:        "target@example.com",
		PasswordHash: "hashed_password",
	})
	require.NoError(t, err)

	t.Run("log admin action", func(t *testing.T) {
		ipAddress := "192.168.1.1"
		userAgent := "Test Agent"
		requestPath := "/api/v1/admin/users"
		requestMethod := "GET"
		statusCode := 200

		err := auditRepo.Log(ctx, admin.AuditLogInput{
			AdminID:        adminUser.ID,
			Action:         "view_users",
			ResourceType:   "user",
			TargetUserID:   &targetUser.ID,
			IPAddress:      &ipAddress,
			UserAgent:      &userAgent,
			RequestPath:    &requestPath,
			RequestMethod:  &requestMethod,
			ResponseStatus: &statusCode,
		})

		require.NoError(t, err)

		// Retrieve logs
		logs, err := auditRepo.GetByAdminID(ctx, adminUser.ID, 10, 0)
		require.NoError(t, err)
		assert.Len(t, logs, 1)

		log := logs[0]
		assert.Equal(t, adminUser.ID, log.AdminID)
		assert.Equal(t, "view_users", log.Action)
		assert.Equal(t, "user", log.ResourceType)
		assert.Equal(t, targetUser.ID, *log.TargetUserID)
		assert.Equal(t, ipAddress, *log.IPAddress)
		assert.Equal(t, userAgent, *log.UserAgent)
		assert.Equal(t, requestPath, *log.RequestPath)
		assert.Equal(t, requestMethod, *log.RequestMethod)
		assert.Equal(t, statusCode, *log.ResponseStatus)
	})

	t.Run("update user role with audit", func(t *testing.T) {
		err := adminService.UpdateUserRole(ctx, adminUser.ID, targetUser.ID, user.RoleAdmin)
		require.NoError(t, err)

		// Verify role was updated
		updatedUser, err := userRepo.GetByID(ctx, targetUser.ID)
		require.NoError(t, err)
		assert.Equal(t, string(user.RoleAdmin), updatedUser.Role)

		// Verify audit log was created
		logs, err := auditRepo.GetByTargetUserID(ctx, targetUser.ID, 10, 0)
		require.NoError(t, err)
		assert.True(t, len(logs) > 0)

		// Find the role update log
		var roleUpdateLog *admin.AuditLog
		for _, log := range logs {
			if log.Action == "update_role" {
				roleUpdateLog = &log
				break
			}
		}

		require.NotNil(t, roleUpdateLog)
		assert.Equal(t, adminUser.ID, roleUpdateLog.AdminID)
		assert.Equal(t, "update_role", roleUpdateLog.Action)
		assert.Equal(t, targetUser.ID, *roleUpdateLog.TargetUserID)
	})

	t.Run("get audit logs by action", func(t *testing.T) {
		logs, err := auditRepo.GetByAction(ctx, "update_role", 10, 0)
		require.NoError(t, err)
		assert.True(t, len(logs) > 0)

		for _, log := range logs {
			assert.Equal(t, "update_role", log.Action)
		}
	})

	t.Run("get audit logs by date range", func(t *testing.T) {
		startDate := time.Now().Add(-1 * time.Hour)
		endDate := time.Now().Add(1 * time.Hour)

		logs, err := auditRepo.GetByDateRange(ctx, startDate, endDate, 10, 0)
		require.NoError(t, err)
		assert.True(t, len(logs) > 0)

		for _, log := range logs {
			assert.True(t, log.CreatedAt.After(startDate))
			assert.True(t, log.CreatedAt.Before(endDate))
		}
	})

	t.Run("count audit logs", func(t *testing.T) {
		count, err := auditRepo.CountByAdminID(ctx, adminUser.ID)
		require.NoError(t, err)
		assert.True(t, count > 0)

		actionCount, err := auditRepo.CountByAction(ctx, "update_role")
		require.NoError(t, err)
		assert.True(t, actionCount > 0)
	})

	t.Run("delete user with audit", func(t *testing.T) {
		// Create another user to delete
		userToDelete, err := userRepo.Create(ctx, user.CreateUserInput{
			Email:        "todelete@example.com",
			PasswordHash: "hashed_password",
		})
		require.NoError(t, err)

		err = adminService.DeleteUser(ctx, adminUser.ID, userToDelete.ID)
		require.NoError(t, err)

		// Verify user was soft deleted
		_, err = userRepo.GetByID(ctx, userToDelete.ID)
		assert.Error(t, err) // Should not be found due to soft delete

		// Verify audit log was created
		logs, err := auditRepo.GetByAction(ctx, "delete_user", 10, 0)
		require.NoError(t, err)
		assert.True(t, len(logs) > 0)

		// Find the delete log for this user
		var deleteLog *admin.AuditLog
		for _, log := range logs {
			if log.TargetUserID != nil && *log.TargetUserID == userToDelete.ID {
				deleteLog = &log
				break
			}
		}

		require.NotNil(t, deleteLog)
		assert.Equal(t, "delete_user", deleteLog.Action)
		assert.Equal(t, adminUser.ID, deleteLog.AdminID)
	})

	t.Run("get audit log by ID", func(t *testing.T) {
		// First create a log
		ipAddress := "192.168.1.1"
		err := auditRepo.Log(ctx, admin.AuditLogInput{
			AdminID:      adminUser.ID,
			Action:       "test_action",
			ResourceType: "test_resource",
			IPAddress:    &ipAddress,
		})
		require.NoError(t, err)

		// Get all logs to find the ID
		logs, err := auditRepo.GetByAdminID(ctx, adminUser.ID, 10, 0)
		require.NoError(t, err)
		require.True(t, len(logs) > 0)

		// Get specific log by ID
		log, err := auditRepo.GetByID(ctx, logs[0].ID)
		require.NoError(t, err)
		assert.Equal(t, logs[0].ID, log.ID)
		assert.Equal(t, logs[0].AdminID, log.AdminID)
		assert.Equal(t, logs[0].Action, log.Action)
	})

	t.Run("pagination of audit logs", func(t *testing.T) {
		// Create multiple logs
		for i := 0; i < 5; i++ {
			ipAddress := "192.168.1.1"
			err := auditRepo.Log(ctx, admin.AuditLogInput{
				AdminID:      adminUser.ID,
				Action:       "pagination_test",
				ResourceType: "test",
				IPAddress:    &ipAddress,
			})
			require.NoError(t, err)
		}

		// Test pagination
		page1, err := auditRepo.GetByAction(ctx, "pagination_test", 2, 0)
		require.NoError(t, err)
		assert.Len(t, page1, 2)

		page2, err := auditRepo.GetByAction(ctx, "pagination_test", 2, 2)
		require.NoError(t, err)
		assert.Len(t, page2, 2)

		// Verify pages have different logs
		assert.NotEqual(t, page1[0].ID, page2[0].ID)
	})
}

func TestUserRoleMethods(t *testing.T) {
	tests := []struct {
		name            string
		role            string
		expectedIsAdmin bool
		expectedIsSuperAdmin bool
	}{
		{
			name:            "user role",
			role:            string(user.RoleUser),
			expectedIsAdmin: false,
			expectedIsSuperAdmin: false,
		},
		{
			name:            "admin role",
			role:            string(user.RoleAdmin),
			expectedIsAdmin: true,
			expectedIsSuperAdmin: false,
		},
		{
			name:            "superadmin role",
			role:            string(user.RoleSuperAdmin),
			expectedIsAdmin: true,
			expectedIsSuperAdmin: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u := &user.User{
				ID:    uuid.New(),
				Email: "test@example.com",
				Role:  tt.role,
			}

			assert.Equal(t, tt.expectedIsAdmin, u.IsAdmin())
			assert.Equal(t, tt.expectedIsSuperAdmin, u.IsSuperAdmin())
		})
	}
}
