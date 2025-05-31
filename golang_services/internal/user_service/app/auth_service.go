package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json" // For NATS message payload
	"errors"
	"log/slog"
	"time"

	"github.com/aradsms/golang_services/internal/platform/messagebroker" // NATS Client
	"github.com/aradsms/golang_services/internal/user_service/domain"
	"github.com/aradsms/golang_services/internal/user_service/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/sha3"
)

// ... (HashPassword, CheckPasswordHash, error variables remain the same) ...
func HashPassword(password string) (string, error) {
    hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        return "", err
    }
    return string(hashedPassword), nil
}
func CheckPasswordHash(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}

var ErrInvalidCredentials = errors.New("invalid username or password")
var ErrEmailExists = errors.New("email already exists")
var ErrUsernameExists = errors.New("username already exists")
var ErrUserNotFound = errors.New("user not found")
var ErrTokenInvalid = errors.New("token is invalid or expired")
var ErrRoleExists = errors.New("role name already exists")
var ErrPermissionExists = errors.New("permission name already exists")
var ErrRoleNotFound = errors.New("role not found")
var ErrPermissionNotFound = errors.New("permission not found")


type AuthConfig struct {
	JWTAccessSecret       string
	JWTRefreshSecret      string
	JWTAccessExpiryHours  int
	JWTRefreshExpiryHours int
}

type AuthService struct {
	userRepo         repository.UserRepository
	roleRepo         repository.RoleRepository
	permRepo         repository.PermissionRepository
	refreshTokenRepo repository.RefreshTokenRepository
	natsClient       *messagebroker.NatsClient // Added NATS client
	config           AuthConfig
	logger           *slog.Logger
}

func NewAuthService(
	userRepo repository.UserRepository,
	roleRepo repository.RoleRepository,
	permRepo repository.PermissionRepository,
	refreshTokenRepo repository.RefreshTokenRepository,
	natsClient *messagebroker.NatsClient, // Added
	config AuthConfig,
	logger *slog.Logger,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		roleRepo:         roleRepo,
		permRepo:         permRepo,
		refreshTokenRepo: refreshTokenRepo,
		natsClient:       natsClient, // Added
		config:           config,
		logger:           logger,
	}
}

func (s *AuthService) RegisterUser(ctx context.Context, username, email, password string, firstName, lastName, phoneNumber string) (*domain.User, error) {
	// ... (existing validation logic for username/email) ...
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
		s.logger.ErrorContext(ctx, "Failed to hash password", "error", err, "username", username)
		return nil, errors.New("failed to process registration")
	}

	defaultRole, err := s.roleRepo.GetByName(ctx, "user")
	if err != nil {
		s.logger.ErrorContext(ctx, "Default role 'user' not found", "error", err)
		return nil, errors.New("system configuration error: default role missing")
	}

	newUser := &domain.User{
		// ... (user fields initialization) ...
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
			return nil, ErrUsernameExists
		}
		return nil, errors.New("failed to save registration")
	}

	// Publish NATS message after successful registration
	if s.natsClient != nil {
		eventPayload := map[string]string{
			"user_id":  createdUser.ID,
			"username": createdUser.Username,
			"email":    createdUser.Email,
		}
		payloadBytes, marshalErr := json.Marshal(eventPayload)
		if marshalErr != nil {
			s.logger.ErrorContext(ctx, "Failed to marshal user created event payload", "error", marshalErr, "userID", createdUser.ID)
		} else {
			subject := "user.created"
			s.logger.InfoContext(ctx, "Publishing NATS message", "subject", subject, "payload", string(payloadBytes))
			if err := s.natsClient.Publish(ctx, subject, payloadBytes); err != nil {
				s.logger.ErrorContext(ctx, "Failed to publish NATS user.created event", "error", err, "userID", createdUser.ID)
				// Non-critical for registration flow, just log
			} else {
                s.logger.InfoContext(ctx, "Successfully published NATS message", "subject", subject, "userID", createdUser.ID)
            }
		}
	} else {
        s.logger.WarnContext(ctx, "NATS client not initialized in AuthService, skipping user.created event publish")
    }

	return createdUser, nil
}

// ... (LoginUser, ValidateAndRefreshTokens, generateAndStoreTokens, API Key methods, Role/Permission methods remain the same) ...
// (Make sure GetUserRepo is still present if needed by gRPC adapter)
func (s *AuthService) GetUserRepo() repository.UserRepository {
    return s.userRepo
}
// LoginUser, ValidateAndRefreshTokens, etc. from previous steps
// ... (rest of the file)
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
        user.FailedLoginAttempts++
        var newLockoutTime *time.Time
        if user.FailedLoginAttempts >= 5 {
            lu := time.Now().Add(15 * time.Minute)
            newLockoutTime = &lu
            user.FailedLoginAttempts = 0
        }
        if err := s.userRepo.UpdateLoginInfo(ctx, user.ID, user.LastLoginAt, user.FailedLoginAttempts, newLockoutTime); err != nil {
             s.logger.ErrorContext(ctx, "Failed to update failed login attempts", "error", err, "userID", user.ID)
        }
        return "", "", ErrInvalidCredentials
    }

    now := time.Now()
    if err := s.userRepo.UpdateLoginInfo(ctx, user.ID, now, 0, nil); err != nil {
       s.logger.ErrorContext(ctx, "Failed to update successful login info", "error", err, "userID", user.ID)
    }
    user.LastLoginAt = &now


    accessToken, refreshToken, err = s.generateAndStoreTokens(ctx, user)
    if err != nil {
        // Error already logged in generateAndStoreTokens
        return "", "", err
    }

    return accessToken, refreshToken, nil
}

func (s *AuthService) ValidateAndRefreshTokens(ctx context.Context, oldRefreshTokenString string) (newAccessToken string, newRefreshToken string, err error) {
    token, err := jwt.Parse(oldRefreshTokenString, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, errors.New("unexpected signing method")
        }
        return []byte(s.config.JWTRefreshSecret), nil
    })

    if err != nil || !token.Valid {
        s.logger.WarnContext(ctx, "Invalid refresh token (parse/validation)", "error", err)
        return "", "", ErrTokenInvalid
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok { return "", "", ErrTokenInvalid }

    userID, _ := claims["sub"].(string)
    refreshTokenID, _ := claims["rti"].(string)

    if userID == "" || refreshTokenID == "" { return "", "", ErrTokenInvalid }

    storedToken, err := s.refreshTokenRepo.GetByID(ctx, refreshTokenID)
    if err != nil {
        if errors.Is(err, repository.ErrRefreshTokenNotFound) {
            s.logger.WarnContext(ctx, "Refresh token not found in DB", "rti", refreshTokenID, "uid", userID)
            return "", "", ErrTokenInvalid
        }
        s.logger.ErrorContext(ctx, "Failed to get refresh token from repo", "error", err, "rti", refreshTokenID)
        return "", "", err
    }

    if storedToken.UserID != userID || time.Now().After(storedToken.ExpiresAt) {
        s.logger.WarnContext(ctx, "Refresh token user mismatch or expired", "dbUID", storedToken.UserID, "jwtUID", userID, "dbExp", storedToken.ExpiresAt)
        if err := s.refreshTokenRepo.DeleteByUserID(ctx, userID); err != nil { // Invalidate family
             s.logger.ErrorContext(ctx, "Failed to delete token family on suspicious refresh", "error", err, "uid", userID)
        }
        return "", "", ErrTokenInvalid
    }

    if err := s.refreshTokenRepo.Delete(ctx, storedToken.ID); err != nil {
        s.logger.ErrorContext(ctx, "Failed to delete old refresh token", "error", err, "rti", storedToken.ID)
        return "", "", errors.New("failed to rotate refresh token")
    }

    user, err := s.userRepo.GetByID(ctx, userID)
    if err != nil {
        s.logger.ErrorContext(ctx, "User not found during token refresh", "error", err, "uid", userID)
        return "", "", ErrTokenInvalid
    }

    newAccessToken, newRefreshToken, err = s.generateAndStoreTokens(ctx, user)
    if err != nil { return "", "", err }

    return newAccessToken, newRefreshToken, nil
}

func (s *AuthService) generateAndStoreTokens(ctx context.Context, user *domain.User) (string, string, error) {
    accessTokenID := uuid.NewString()
    accessClaims := jwt.MapClaims{
        "sub": user.ID, "unm": user.Username, "uid": user.ID, "adm": user.IsAdmin, "rol": user.RoleID,
        "jti": accessTokenID,
        "exp": time.Now().Add(time.Hour * time.Duration(s.config.JWTAccessExpiryHours)).Unix(),
        "iat": time.Now().Unix(), "iss": "aradsms-user-service",
    }
    accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(s.config.JWTAccessSecret))
    if err != nil {
        s.logger.ErrorContext(ctx, "Failed to sign access token", "error", err, "userID", user.ID)
        return "", "", errors.New("token generation error")
    }

    refreshTokenID := uuid.NewString() // DB primary key for the refresh token record
    refreshTokenJWTID := uuid.NewString() // JTI for the JWT refresh token itself

    refreshClaims := jwt.MapClaims{
        "sub": user.ID,
        "rti": refreshTokenID,    // This is the ID of the DB record for this refresh token.
        "jti": refreshTokenJWTID, // This is the ID of this specific JWT refresh token.
        "exp": time.Now().Add(time.Hour * time.Duration(s.config.JWTRefreshExpiryHours)).Unix(),
        "iat": time.Now().Unix(), "iss": "aradsms-user-service",
    }
    refreshTokenActualJWT, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(s.config.JWTRefreshSecret))
    if err != nil {
        s.logger.ErrorContext(ctx, "Failed to sign refresh JWT", "error", err, "userID", user.ID)
        return "", "", errors.New("token generation error")
    }

    dbRefreshToken := &domain.RefreshToken{
        ID:        refreshTokenID, // Storing the 'rti' claim value as the DB record ID
        UserID:    user.ID,
        TokenHash: "", // Not hashing the JWT. Presence + Expiry in DB means it's valid.
        ExpiresAt: time.Unix(refreshClaims["exp"].(int64), 0),
    }
    if err := s.refreshTokenRepo.Create(ctx, dbRefreshToken); err != nil {
        s.logger.ErrorContext(ctx, "Failed to store refresh token metadata", "error", err, "userID", user.ID)
        return "", "", errors.New("session persistence error")
    }
    return accessToken, refreshTokenActualJWT, nil
}


func GenerateAPIKey() (plainTextKey string, hashedKey string, err error) { /* ... */
    randomBytes := make([]byte, 32)
    if _, err := rand.Read(randomBytes); err != nil { return "", "", err }
    plainTextKey = base64.URLEncoding.EncodeToString(randomBytes)
    hash := sha3.Sum256([]byte(plainTextKey))
    hashedKey = base64.URLEncoding.EncodeToString(hash[:])
    return plainTextKey, hashedKey, nil
}
func (s *AuthService) AssignAPIKeyToUser(ctx context.Context, userID string) (plainTextKey string, err error) { /* ... */
    user, err := s.userRepo.GetByID(ctx, userID)
    if err != nil { return "", err }
    plainTextKey, apiKeyHash, err := GenerateAPIKey()
    if err != nil {
        s.logger.ErrorContext(ctx, "Failed to generate API key", "error", err)
        return "", errors.New("API key generation failed")
    }
    user.APIKey = apiKeyHash
    if err := s.userRepo.Update(ctx, user); err != nil {
        s.logger.ErrorContext(ctx, "Failed to update user with API key hash", "error", err, "userID", userID)
        return "", errors.New("failed to assign API key")
    }
    return plainTextKey, nil
}
func (s *AuthService) ValidateAPIKey(ctx context.Context, plainTextKey string) (*domain.User, error) { /* ... */
    hash := sha3.Sum256([]byte(plainTextKey))
    hashedKeyToValidate := base64.URLEncoding.EncodeToString(hash[:])
    user, err := s.userRepo.GetByAPIKeyHash(ctx, hashedKeyToValidate)
    if err != nil {
        if errors.Is(err, repository.ErrUserNotFound) { return nil, ErrTokenInvalid }
        s.logger.ErrorContext(ctx, "Error fetching user by API key hash", "error", err)
        return nil, err
    }
    if !user.IsActive { return nil, errors.New("user account is not active") }
    return user, nil
}
func (s *AuthService) CreateRole(ctx context.Context, name, description string) (*domain.Role, error) { /* ... */
    if _, err := s.roleRepo.GetByName(ctx, name); err == nil { return nil, ErrRoleExists } else if !errors.Is(err, repository.ErrRoleNotFound) { return nil, err }
    newRole := &domain.Role{Name: name, Description: description}
    return s.roleRepo.Create(ctx, newRole)
}
func (s *AuthService) GetRole(ctx context.Context, roleID string) (*domain.Role, error) { /* ... */
    role, err := s.roleRepo.GetByID(ctx, roleID)
    if err != nil { if errors.Is(err, repository.ErrRoleNotFound) { return nil, ErrRoleNotFound }; return nil, err }
    return role, nil
}
func (s *AuthService) GetAllRoles(ctx context.Context) ([]domain.Role, error) { /* ... */ return s.roleRepo.GetAll(ctx) }
func (s *AuthService) CreatePermission(ctx context.Context, name, description string) (*domain.Permission, error) { /* ... */
    if _, err := s.permRepo.GetByName(ctx, name); err == nil { return nil, ErrPermissionExists } else if !errors.Is(err, repository.ErrPermissionNotFound) { return nil, err }
    newPerm := &domain.Permission{Name: name, Description: description}
    return s.permRepo.Create(ctx, newPerm)
}
func (s *AuthService) GetAllPermissions(ctx context.Context) ([]domain.Permission, error) { /* ... */ return s.permRepo.GetAll(ctx) }
func (s *AuthService) AssignPermissionToRole(ctx context.Context, roleID, permissionID string) error { /* ... */
    // This had a bug, fixed:
    if _, err := s.roleRepo.GetByID(ctx, roleID); err != nil {
        if errors.Is(err, repository.ErrRoleNotFound) { return ErrRoleNotFound }
        return err
    }
    if _, err := s.permRepo.GetByID(ctx, permissionID); err != nil {
        if errors.Is(err, repository.ErrPermissionNotFound) { return ErrPermissionNotFound }
        return err
    }
    return s.roleRepo.AddPermissionToRole(ctx, roleID, permissionID)
}
func (s *AuthService) RemovePermissionFromRole(ctx context.Context, roleID, permissionID string) error { /* ... */ return s.roleRepo.RemovePermissionFromRole(ctx, roleID, permissionID) }
func (s *AuthService) GetUserPermissions(ctx context.Context, userID string) ([]domain.Permission, error) { /* ... */
    user, err := s.userRepo.GetByID(ctx, userID)
    if err != nil { return nil, err }
    if user.RoleID == "" { return []domain.Permission{}, nil }
    return s.roleRepo.GetPermissionsForRole(ctx, user.RoleID)
}
