package app_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
    "io"
    "time"

	"github.com/aradsms/golang_services/internal/platform/messagebroker"
	"github.com/aradsms/golang_services/internal/user_service/app"
	"github.com/aradsms/golang_services/internal/user_service/domain"
	"github.com/aradsms/golang_services/internal/user_service/repository"
	"github.com/nats-io/nats.go" // For nats.Msg, not directly used in mock publish
)

// MockNatsClient for testing NATS publishing
type MockNatsClient struct {
	messagebroker.NatsClient // Embed to avoid implementing all methods
	PublishFunc func(ctx context.Context, subject string, data []byte) error
    PublishedMessages map[string][]byte // Store published messages for inspection
}

func NewMockNatsClient() *MockNatsClient {
    return &MockNatsClient{
        PublishedMessages: make(map[string][]byte),
    }
}

func (m *MockNatsClient) Publish(ctx context.Context, subject string, data []byte) error {
    if m.PublishFunc != nil {
        return m.PublishFunc(ctx, subject, data)
    }
    if m.PublishedMessages == nil { // Ensure map is initialized
        m.PublishedMessages = make(map[string][]byte)
    }
    m.PublishedMessages[subject] = data // Store the last message for the subject
    return nil
}
// We only need to mock Publish for this test, other NatsClient methods can use embedded real ones (which will fail if not configured)
// or be individually mocked if specific tests need them.


// MockUserRepository for AuthService tests
type MockUserRepository struct {
	// repository.UserRepository // Embedding this would require mocking all its methods if not careful
    // Instead, list only the methods this specific test needs, or the AuthService uses.
	CreateFunc func(ctx context.Context, user *domain.User) (*domain.User, error)
    GetByUsernameFunc func(ctx context.Context, username string) (*domain.User, error)
    GetByEmailFunc func(ctx context.Context, email string) (*domain.User, error)
    GetByIDFunc func(ctx context.Context, id string) (*domain.User, error)
    GetByAPIKeyHashFunc func(ctx context.Context, apiKeyHash string) (*domain.User, error)
    UpdateFunc func(ctx context.Context, user *domain.User) error
    UpdateLoginInfoFunc func(ctx context.Context, id string, loginTime time.Time, failedAttempts int, lockoutUntil *time.Time) error
    DeleteFunc func(ctx context.Context, id string) error
}

// Implement repository.UserRepository interface for MockUserRepository
func (m *MockUserRepository) Create(ctx context.Context, user *domain.User) (*domain.User, error) {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, user)
	}
	user.ID = "mock-id-" + user.Username // Simulate ID generation
	return user, nil
}
func (m *MockUserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
    if m.GetByUsernameFunc != nil {
        return m.GetByUsernameFunc(ctx, username)
    }
    return nil, repository.ErrUserNotFound // Default to not found
}
func (m *MockUserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
    if m.GetByEmailFunc != nil {
        return m.GetByEmailFunc(ctx, email)
    }
    return nil, repository.ErrUserNotFound // Default to not found
}
func (m *MockUserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
    if m.GetByIDFunc != nil { return m.GetByIDFunc(ctx, id) }
    return nil, repository.ErrUserNotFound
}
func (m *MockUserRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.User, error) {
    if m.GetByAPIKeyHashFunc != nil { return m.GetByAPIKeyHashFunc(ctx, apiKeyHash) }
    return nil, repository.ErrUserNotFound
}
func (m *MockUserRepository) Update(ctx context.Context, user *domain.User) error {
    if m.UpdateFunc != nil { return m.UpdateFunc(ctx, user) }
    return nil
}
func (m *MockUserRepository) UpdateLoginInfo(ctx context.Context, id string, loginTime time.Time, failedAttempts int, lockoutUntil *time.Time) error {
    if m.UpdateLoginInfoFunc != nil { return m.UpdateLoginInfoFunc(ctx, id, loginTime, failedAttempts, lockoutUntil) }
    return nil
}
func (m *MockUserRepository) Delete(ctx context.Context, id string) error {
    if m.DeleteFunc != nil { return m.DeleteFunc(ctx, id) }
    return nil
}


// MockRoleRepository
type MockRoleRepository struct {
    // repository.RoleRepository // Embed for full interface, or list methods used
    GetByNameFunc func(ctx context.Context, name string) (*domain.Role, error)
    GetByIDFunc func(ctx context.Context, id string) (*domain.Role, error)
    GetAllFunc func(ctx context.Context) ([]domain.Role, error)
    CreateFunc func(ctx context.Context, role *domain.Role) (*domain.Role, error)
    UpdateFunc func(ctx context.Context, role *domain.Role) error
    DeleteFunc func(ctx context.Context, id string) error
    AddPermissionToRoleFunc func(ctx context.Context, roleID string, permissionID string) error
    RemovePermissionFromRoleFunc func(ctx context.Context, roleID string, permissionID string) error
    GetPermissionsForRoleFunc func(ctx context.Context, roleID string) ([]domain.Permission, error)
}
// Implement repository.RoleRepository for MockRoleRepository
func (m *MockRoleRepository) GetByName(ctx context.Context, name string) (*domain.Role, error) {
    if m.GetByNameFunc != nil { return m.GetByNameFunc(ctx, name) }
    if name == "user" { return &domain.Role{ID: "role-user-id", Name: "user"}, nil }
    return nil, repository.ErrRoleNotFound
}
func (m *MockRoleRepository) GetByID(ctx context.Context, id string) (*domain.Role, error) {
    if m.GetByIDFunc != nil { return m.GetByIDFunc(ctx, id) }
    return nil, repository.ErrRoleNotFound
}
func (m *MockRoleRepository) GetAll(ctx context.Context) ([]domain.Role, error) {
    if m.GetAllFunc != nil { return m.GetAllFunc(ctx) }
    return nil, nil
}
func (m *MockRoleRepository) Create(ctx context.Context, role *domain.Role) (*domain.Role, error) {
    if m.CreateFunc != nil { return m.CreateFunc(ctx, role) }
    return role, nil
}
func (m *MockRoleRepository) Update(ctx context.Context, role *domain.Role) error {
    if m.UpdateFunc != nil { return m.UpdateFunc(ctx, role) }
    return nil
}
func (m *MockRoleRepository) Delete(ctx context.Context, id string) error {
    if m.DeleteFunc != nil { return m.DeleteFunc(ctx, id) }
    return nil
}
func (m *MockRoleRepository) AddPermissionToRole(ctx context.Context, roleID string, permissionID string) error {
    if m.AddPermissionToRoleFunc != nil { return m.AddPermissionToRoleFunc(ctx, roleID, permissionID) }
    return nil
}
func (m *MockRoleRepository) RemovePermissionFromRole(ctx context.Context, roleID string, permissionID string) error {
    if m.RemovePermissionFromRoleFunc != nil { return m.RemovePermissionFromRoleFunc(ctx, roleID, permissionID) }
    return nil
}
func (m *MockRoleRepository) GetPermissionsForRole(ctx context.Context, roleID string) ([]domain.Permission, error) {
    if m.GetPermissionsForRoleFunc != nil { return m.GetPermissionsForRoleFunc(ctx, roleID) }
    return nil, nil
}

// MockPermissionRepository
type MockPermissionRepository struct {
    // repository.PermissionRepository
    GetByNameFunc func(ctx context.Context, name string) (*domain.Permission, error)
    GetByIDFunc func(ctx context.Context, id string) (*domain.Permission, error)
    GetAllFunc func(ctx context.Context) ([]domain.Permission, error)
    CreateFunc func(ctx context.Context, perm *domain.Permission) (*domain.Permission, error)
}
func (m *MockPermissionRepository) GetByName(ctx context.Context, name string) (*domain.Permission, error) {
    if m.GetByNameFunc != nil { return m.GetByNameFunc(ctx, name)}
    return nil, repository.ErrPermissionNotFound
}
func (m *MockPermissionRepository) GetByID(ctx context.Context, id string) (*domain.Permission, error) {
    if m.GetByIDFunc != nil { return m.GetByIDFunc(ctx, id)}
    return nil, repository.ErrPermissionNotFound
}
func (m *MockPermissionRepository) GetAll(ctx context.Context) ([]domain.Permission, error) {
    if m.GetAllFunc != nil { return m.GetAllFunc(ctx)}
    return nil, nil
}
func (m *MockPermissionRepository) Create(ctx context.Context, perm *domain.Permission) (*domain.Permission, error) {
    if m.CreateFunc != nil { return m.CreateFunc(ctx, perm)}
    return perm, nil
}

// MockRefreshTokenRepository
type MockRefreshTokenRepository struct {
    // repository.RefreshTokenRepository
    CreateFunc func(ctx context.Context, token *domain.RefreshToken) error
    GetByIDFunc func(ctx context.Context, id string) (*domain.RefreshToken, error) // Added GetByID
    GetByTokenHashFunc func(ctx context.Context, tokenHash string) (*domain.RefreshToken, error)
    DeleteFunc func(ctx context.Context, id string) error
    DeleteByUserIDFunc func(ctx context.Context, userID string) error
}
func (m *MockRefreshTokenRepository) Create(ctx context.Context, token *domain.RefreshToken) error {
    if m.CreateFunc != nil { return m.CreateFunc(ctx, token) }
    return nil
}
func (m *MockRefreshTokenRepository) GetByID(ctx context.Context, id string) (*domain.RefreshToken, error) {
    if m.GetByIDFunc != nil { return m.GetByIDFunc(ctx, id) }
    return nil, repository.ErrRefreshTokenNotFound
}
func (m *MockRefreshTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
    if m.GetByTokenHashFunc != nil { return m.GetByTokenHashFunc(ctx, tokenHash) }
    return nil, repository.ErrRefreshTokenNotFound
}
func (m *MockRefreshTokenRepository) Delete(ctx context.Context, id string) error {
    if m.DeleteFunc != nil { return m.DeleteFunc(ctx, id) }
    return nil
}
func (m *MockRefreshTokenRepository) DeleteByUserID(ctx context.Context, userID string) error {
    if m.DeleteByUserIDFunc != nil { return m.DeleteByUserIDFunc(ctx, userID) }
    return nil
}


func TestAuthService_RegisterUser_NatsPublish(t *testing.T) {
    logger := slog.New(slog.NewTextHandler(io.Discard, nil))
    mockNats := NewMockNatsClient()

    mockUserRepo := &MockUserRepository{
        GetByUsernameFunc: func(ctx context.Context, username string) (*domain.User, error) {
            return nil, repository.ErrUserNotFound
        },
        GetByEmailFunc: func(ctx context.Context, email string) (*domain.User, error) {
            return nil, repository.ErrUserNotFound
        },
        CreateFunc: func(ctx context.Context, user *domain.User) (*domain.User, error) {
            user.ID = "test-user-123" // Ensure ID is set for event
            return user, nil
        },
    }
    mockRoleRepo := &MockRoleRepository{
         GetByNameFunc: func(ctx context.Context, name string) (*domain.Role, error) {
            return &domain.Role{ID: "default-role-id", Name: "user"}, nil
        },
    }
    mockPermRepo := &MockPermissionRepository{} // Assign empty mocks if not used by specific test path
    mockRefreshTokenRepo := &MockRefreshTokenRepository{}


    authCfg := app.AuthConfig{JWTAccessExpiryHours: 1, JWTRefreshExpiryHours: 24}
    authService := app.NewAuthService(mockUserRepo, mockRoleRepo, mockPermRepo, mockRefreshTokenRepo, mockNats, authCfg, logger)

    _, err := authService.RegisterUser(context.Background(), "natsuser", "nats@example.com", "password", "", "", "")
    if err != nil {
        t.Fatalf("RegisterUser failed: %v", err)
    }

    // Check if NATS message was "published"
    if publishedData, ok := mockNats.PublishedMessages["user.created"]; ok {
        var eventPayload map[string]string
        if err := json.Unmarshal(publishedData, &eventPayload); err != nil {
            t.Fatalf("Failed to unmarshal NATS message payload: %v", err)
        }
        if eventPayload["user_id"] != "test-user-123" {
            t.Errorf("Expected user_id 'test-user-123' in NATS message, got '%s'", eventPayload["user_id"])
        }
        if eventPayload["username"] != "natsuser" {
            t.Errorf("Expected username 'natsuser' in NATS message, got '%s'", eventPayload["username"])
        }
    } else {
        t.Errorf("NATS message for 'user.created' was not published")
    }
}
