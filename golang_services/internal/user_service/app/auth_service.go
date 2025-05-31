package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog" // Or your platform logger
	"time"

	"github.com/aradsms/golang_services/internal/user_service/domain"
	"github.com/aradsms/golang_services/internal/user_service/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid" // For generating API keys or other IDs if needed
	"golang.org/x/crypto/bcrypt" // Moved from password.go for direct use here or keep in password.go
	"golang.org/x/crypto/sha3"
)

// Password hashing functions (can be kept in password.go or moved here if preferred)
// HashPassword generates a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// CheckPasswordHash compares a plain text password with a bcrypt hash.
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}


var ErrInvalidCredentials = errors.New("invalid username or password")
var ErrEmailExists = errors.New("email already exists")
var ErrUsernameExists = errors.New("username already exists")
var ErrUserNotFound = errors.New("user not found") // Can reuse from repository
var ErrTokenInvalid = errors.New("token is invalid or expired")
var ErrRoleExists = errors.New("role name already exists")
var ErrPermissionExists = errors.New("permission name already exists")
var ErrRoleNotFound = errors.New("role not found")
var ErrPermissionNotFound = errors.New("permission not found")


// AuthConfig holds configuration for the AuthService, e.g., JWT secrets and expiry.
type AuthConfig struct {
	JWTAccessSecret      string
	JWTRefreshSecret     string
	JWTAccessExpiryHours int
	JWTRefreshExpiryHours int
}

// AuthService provides core authentication and user management logic.
type AuthService struct {
	userRepo         repository.UserRepository
	roleRepo         repository.RoleRepository
	permRepo         repository.PermissionRepository // Added PermissionRepository
	refreshTokenRepo repository.RefreshTokenRepository
	config           AuthConfig
	logger           *slog.Logger
}

// NewAuthService creates a new AuthService.
func NewAuthService(
	userRepo repository.UserRepository,
	roleRepo repository.RoleRepository,
	permRepo repository.PermissionRepository, // Added
	refreshTokenRepo repository.RefreshTokenRepository,
	config AuthConfig,
	logger *slog.Logger,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		roleRepo:         roleRepo,
		permRepo:         permRepo, // Added
		refreshTokenRepo: refreshTokenRepo,
		config:           config,
		logger:           logger,
	}
}

// RegisterUser registers a new user.
func (s *AuthService) RegisterUser(ctx context.Context, username, email, password string, firstName, lastName, phoneNumber string) (*domain.User, error) {
	// Check if username or email already exists
	existingUser, err := s.userRepo.GetByUsername(ctx, username)
	if err == nil && existingUser != nil {
		return nil, ErrUsernameExists
	}
	if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
        s.logger.ErrorContext(ctx, "Error checking username existence", "error", err, "username", username)
		return nil, err
	}

	existingUserByEmail, err := s.userRepo.GetByEmail(ctx, email)
	if err == nil && existingUserByEmail != nil {
		return nil, ErrEmailExists
	}
    if err != nil && !errors.Is(err, repository.ErrUserNotFound) {
        s.logger.ErrorContext(ctx, "Error checking email existence", "error", err, "email", email)
		return nil, err
	}


	hashedPassword, err := HashPassword(password)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to hash password during registration", "error", err, "username", username)
		return nil, errors.New("failed to process registration")
	}

	defaultRole, err := s.roleRepo.GetByName(ctx, "user") // Assuming "user" role exists
	if err != nil {
		s.logger.ErrorContext(ctx, "Default role 'user' not found", "error", err)
		return nil, errors.New("system configuration error: default role missing")
	}

	newUser := &domain.User{
		Username:       username,
		Email:          email,
		HashedPassword: hashedPassword,
		FirstName:      firstName,
		LastName:       lastName,
		PhoneNumber:    phoneNumber,
		IsActive:       true,
		IsAdmin:        false,
		RoleID:         defaultRole.ID,
		CreditBalance:  0,
		CurrencyCode:   "USD",
	}

	createdUser, err := s.userRepo.Create(ctx, newUser)
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to create user in repository", "error", err, "username", username)
		if errors.Is(err, repository.ErrDuplicateUser) {
			return nil, ErrUsernameExists // Or ErrEmailExists
		}
		return nil, errors.New("failed to save registration")
	}

	return createdUser, nil
}

// LoginUser authenticates a user and returns access and refresh JWTs.
func (s *AuthService) LoginUser(ctx context.Context, username, password string) (accessToken string, refreshToken string, err error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return "", "", ErrInvalidCredentials
		}
		s.logger.ErrorContext(ctx, "Error fetching user by username", "error", err, "username", username)
		return "", "", err
	}

	if !user.IsActive {
        s.logger.WarnContext(ctx, "Login attempt for inactive user", "username", username, "userID", user.ID)
		return "", "", errors.New("user account is not active")
	}

    if user.LockoutUntil != nil && time.Now().Before(*user.LockoutUntil) {
        s.logger.WarnContext(ctx, "Login attempt for locked out user", "username", username, "userID", user.ID, "lockout_until", *user.LockoutUntil)
        return "", "", errors.New("account is temporarily locked")
    }

	if !CheckPasswordHash(password, user.HashedPassword) {
        // Increment failed attempts & potentially lock account
        user.FailedLoginAttempts++
        // Example: lock for 15 mins after 5 failed attempts
        var newLockoutTime *time.Time
        if user.FailedLoginAttempts >= 5 {
            lu := time.Now().Add(15 * time.Minute)
            newLockoutTime = &lu
            user.FailedLoginAttempts = 0 // Reset after locking
        }
        if err := s.userRepo.UpdateLoginInfo(ctx, user.ID, user.LastLoginAt, user.FailedLoginAttempts, newLockoutTime); err != nil {
             s.logger.ErrorContext(ctx, "Failed to update failed login attempts", "error", err, "userID", user.ID)
        }
		return "", "", ErrInvalidCredentials
	}

    // Reset failed attempts on successful login & update last login time
    now := time.Now()
    if err := s.userRepo.UpdateLoginInfo(ctx, user.ID, now, 0, nil); err != nil {
       s.logger.ErrorContext(ctx, "Failed to update successful login info", "error", err, "userID", user.ID)
        // Non-critical, proceed with login
    }
    user.LastLoginAt = &now // Update in-memory user object if needed elsewhere in this request


	accessToken, refreshToken, err = s.generateAndStoreTokens(ctx, user)
    if err != nil {
        // It's better to return three distinct values to match the signature
        return "", "", err
    }

	return accessToken, refreshToken, nil
}

// ValidateAndRefreshTokens validates a refresh token and issues new access and refresh tokens.
func (s *AuthService) ValidateAndRefreshTokens(ctx context.Context, oldRefreshTokenString string) (newAccessToken string, newRefreshToken string, err error) {
	token, err := jwt.Parse(oldRefreshTokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(s.config.JWTRefreshSecret), nil
	})

	if err != nil || !token.Valid {
		s.logger.WarnContext(ctx, "Invalid refresh token presented (parse/validation)", "error", err)
		return "", "", ErrTokenInvalid
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", "", ErrTokenInvalid
	}

	userID, _ := claims["sub"].(string)
	refreshTokenID, _ := claims["rti"].(string) // Refresh Token ID (from JWT claims)

	if userID == "" || refreshTokenID == "" {
		return "", "", ErrTokenInvalid
	}

	// 2. Check against stored refresh tokens by ID
	storedToken, err := s.refreshTokenRepo.GetByID(ctx, refreshTokenID) // Assuming GetByID takes the JWT's "rti" which is stored as ID
	if err != nil {
		if errors.Is(err, repository.ErrRefreshTokenNotFound) {
			s.logger.WarnContext(ctx, "Refresh token not found in DB", "refreshTokenID", refreshTokenID, "userID", userID)
			return "", "", ErrTokenInvalid
		}
		s.logger.ErrorContext(ctx, "Failed to get refresh token from repo", "error", err, "refreshTokenID", refreshTokenID)
		return "", "", err
	}

	if storedToken.UserID != userID || time.Now().After(storedToken.ExpiresAt) {
		s.logger.WarnContext(ctx, "Refresh token mismatch or expired", "dbUserID", storedToken.UserID, "jwtUserID", userID, "dbExpires", storedToken.ExpiresAt)
		// Important: If a token is attempted to be used after expiry or by wrong user,
		// or if it's simply not found (already used), invalidate this token family.
		if err := s.refreshTokenRepo.DeleteByUserID(ctx, userID); err != nil {
             s.logger.ErrorContext(ctx, "Failed to delete token family on suspicious refresh attempt", "error", err, "userID", userID)
        }
		return "", "", ErrTokenInvalid
	}

	// 3. Strict rotation: Delete the used refresh token
	if err := s.refreshTokenRepo.Delete(ctx, storedToken.ID); err != nil {
		s.logger.ErrorContext(ctx, "Failed to delete old refresh token during refresh", "error", err, "refreshTokenID", storedToken.ID)
		// If deletion fails, it's a problem. Could allow refresh but log heavily, or deny. For stricter security, deny.
		return "", "", errors.New("failed to rotate refresh token")
	}

	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		s.logger.ErrorContext(ctx, "User not found during token refresh", "error", err, "userID", userID)
		return "", "", ErrTokenInvalid
	}

	newAccessToken, newRefreshToken, err = s.generateAndStoreTokens(ctx, user)
	if err != nil {
		return "", "", err
	}

	return newAccessToken, newRefreshToken, nil
}

func (s *AuthService) generateAndStoreTokens(ctx context.Context, user *domain.User) (string, string, error) {
	accessTokenID := uuid.NewString()
	accessClaims := jwt.MapClaims{
		"sub": user.ID,
		"unm": user.Username,
		"uid": user.ID,
		"adm": user.IsAdmin,
		"rol": user.RoleID,
        "jti": accessTokenID, // JWT ID for access token
		"exp": time.Now().Add(time.Hour * time.Duration(s.config.JWTAccessExpiryHours)).Unix(),
		"iat": time.Now().Unix(),
		"iss": "aradsms-user-service",
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(s.config.JWTAccessSecret))
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to sign access token", "error", err, "userID", user.ID)
		return "", "", errors.New("token generation error")
	}

	refreshTokenID := uuid.NewString() // This will be the ID stored in the DB
	// refreshTokenString := uuid.NewString() // This is the opaque token string returned to client // This line is not used if JWT is the refresh token

	refreshClaims := jwt.MapClaims{
		"sub": user.ID,
		"rti": refreshTokenID, // The ID of the DB record for this refresh token session
		"exp": time.Now().Add(time.Hour * time.Duration(s.config.JWTRefreshExpiryHours)).Unix(),
		"iat": time.Now().Unix(),
		"iss": "aradsms-user-service",
	}
	// The JWT refresh token is now just a carrier for the DB record ID (rti) and its own expiry.
	// The actual "secret" part of the refresh token is the opaque string.
	// However, for simplicity, many systems use a self-contained JWT as a refresh token.
	// If using self-contained JWT as refresh token, then rti is the jti of that JWT.
	// Let's stick to self-contained JWT as refresh token for now for simplicity of client handling.
	// The "rti" claim will be its own JTI (JWT ID).
	refreshTokenActualJWT, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(s.config.JWTRefreshSecret))
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to sign refresh JWT", "error", err, "userID", user.ID)
		return "", "", errors.New("token generation error")
	}

	// Store metadata about the refresh token session (identified by its JTI)
	dbRefreshToken := &domain.RefreshToken{
		ID:        refreshTokenID, // This is the JWT's "rti" (which is its JTI)
		UserID:    user.ID,
		TokenHash: "", // Not storing hash of JWT. Presence in DB means it's valid until expired or deleted.
		ExpiresAt: time.Unix(refreshClaims["exp"].(int64), 0),
	}
	if err := s.refreshTokenRepo.Create(ctx, dbRefreshToken); err != nil {
		s.logger.ErrorContext(ctx, "Failed to store refresh token metadata", "error", err, "userID", user.ID)
		return "", "", errors.New("session persistence error")
	}
	return accessToken, refreshTokenActualJWT, nil
}

func GenerateAPIKey() (plainTextKey string, hashedKey string, err error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", err
	}
	plainTextKey = base64.URLEncoding.EncodeToString(randomBytes)
	hash := sha3.Sum256([]byte(plainTextKey))
	hashedKey = base64.URLEncoding.EncodeToString(hash[:])
	return plainTextKey, hashedKey, nil
}

func (s *AuthService) AssignAPIKeyToUser(ctx context.Context, userID string) (plainTextKey string, err error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}

	plainTextKey, apiKeyHash, err := GenerateAPIKey()
	if err != nil {
		s.logger.ErrorContext(ctx, "Failed to generate API key", "error", err)
		return "", errors.New("API key generation failed")
	}

	user.APIKey = apiKeyHash // Assuming domain.User.APIKey stores the hash
	if err := s.userRepo.Update(ctx, user); err != nil { // Ensure Update method can update APIKeyHash
		s.logger.ErrorContext(ctx, "Failed to update user with API key hash", "error", err, "userID", userID)
		return "", errors.New("failed to assign API key")
	}
	return plainTextKey, nil
}

func (s *AuthService) ValidateAPIKey(ctx context.Context, plainTextKey string) (*domain.User, error) {
	hash := sha3.Sum256([]byte(plainTextKey))
	hashedKeyToValidate := base64.URLEncoding.EncodeToString(hash[:])

	user, err := s.userRepo.GetByAPIKeyHash(ctx, hashedKeyToValidate)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrTokenInvalid // Generic error for invalid key
		}
		s.logger.ErrorContext(ctx, "Error fetching user by API key hash", "error", err)
		return nil, err
	}
	if !user.IsActive {
		return nil, errors.New("user account is not active")
	}
	// Optionally, update user.LastLoginAt or similar audit field for API key usage
	return user, nil
}

// --- Role Management ---
func (s *AuthService) CreateRole(ctx context.Context, name, description string) (*domain.Role, error) {
	if _, err := s.roleRepo.GetByName(ctx, name); err == nil {
		return nil, ErrRoleExists
	} else if !errors.Is(err, repository.ErrRoleNotFound) {
        return nil, err
    }


	newRole := &domain.Role{
		Name:        name,
		Description: description,
	}
	return s.roleRepo.Create(ctx, newRole)
}

func (s *AuthService) GetRole(ctx context.Context, roleID string) (*domain.Role, error) {
	role, err := s.roleRepo.GetByID(ctx, roleID)
    if err != nil {
        if errors.Is(err, repository.ErrRoleNotFound) {
            return nil, ErrRoleNotFound
        }
        return nil, err
    }
    // Optionally load permissions here or leave it to a separate method/resolver
    // permissions, err := s.roleRepo.GetPermissionsForRole(ctx, roleID)
    // if err != nil {
    //    return nil, err
    // }
    // role.Permissions = permissions
	return role, nil
}

func (s *AuthService) GetAllRoles(ctx context.Context) ([]domain.Role, error) {
    return s.roleRepo.GetAll(ctx)
}


// --- Permission Management ---
func (s *AuthService) CreatePermission(ctx context.Context, name, description string) (*domain.Permission, error) {
	if _, err := s.permRepo.GetByName(ctx, name); err == nil {
		return nil, ErrPermissionExists
	} else if !errors.Is(err, repository.ErrPermissionNotFound) {
        return nil, err
    }

	newPerm := &domain.Permission{
		Name:        name,
		Description: description,
	}
	return s.permRepo.Create(ctx, newPerm)
}

func (s *AuthService) GetAllPermissions(ctx context.Context) ([]domain.Permission, error) {
    return s.permRepo.GetAll(ctx)
}

// --- Role-Permission Assignment ---
func (s *AuthService) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error {
	// Validate role and permission exist
	if _, err := s.roleRepo.GetByID(ctx, roleID); err != nil {
        if errors.Is(err, repository.ErrRoleNotFound) {
            return ErrRoleNotFound
        }
		return err
	}
	if _, err := s.permRepo.GetByID(ctx, permissionID); err != nil {
        if errors.Is(err, repository.ErrPermissionNotFound) {
            return ErrPermissionNotFound
        }
		return err
	}
	return s.roleRepo.AddPermissionToRole(ctx, roleID, permissionID)
}

func (s *AuthService) RemovePermissionFromRole(ctx context.Context, roleID, permissionID string) error {
	return s.roleRepo.RemovePermissionFromRole(ctx, roleID, permissionID)
}

// GetUserPermissions retrieves all permissions for a given user (via their role).
func (s *AuthService) GetUserPermissions(ctx context.Context, userID string) ([]domain.Permission, error) {
    user, err := s.userRepo.GetByID(ctx, userID)
    if err != nil {
        return nil, err // Handles ErrUserNotFound from repo
    }
    if user.RoleID == "" {
        return []domain.Permission{}, nil // No role, no permissions
    }
    return s.roleRepo.GetPermissionsForRole(ctx, user.RoleID)
}

// GetUserRepo returns the underlying UserRepository.
// This is used by the gRPC adapter to fetch full user details for IsActive check.
// Consider if a more specific app method is better long-term.
func (s *AuthService) GetUserRepo() repository.UserRepository {
	return s.userRepo
}
