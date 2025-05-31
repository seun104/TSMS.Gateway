package http

// RegisterRequest defines the structure for user registration.
type RegisterRequest struct {
	Username    string `json:"username" validate:"required,min=3,max=50"`
	Email       string `json:"email" validate:"required,email"`
	Password    string `json:"password" validate:"required,min=8,max=100"`
	FirstName   string `json:"first_name,omitempty"`
	LastName    string `json:"last_name,omitempty"`
	PhoneNumber string `json:"phone_number,omitempty"`
}

// LoginRequest defines the structure for user login.
type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// LoginResponse defines the structure for a successful login.
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
}

// RefreshTokenRequest defines the structure for refreshing a token.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// UserProfileResponse defines the structure for the /users/me endpoint.
type UserProfileResponse struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	FirstName   string   `json:"first_name,omitempty"`
	LastName    string   `json:"last_name,omitempty"`
	PhoneNumber string   `json:"phone_number,omitempty"`
	RoleID      string   `json:"role_id,omitempty"`
	IsAdmin     bool     `json:"is_admin"`
	IsActive    bool     `json:"is_active"`
	Permissions []string `json:"permissions,omitempty"`
}

// GenericErrorResponse for API errors
type GenericErrorResponse struct {
    Error   string `json:"error"`
    Details string `json:"details,omitempty"`
}
