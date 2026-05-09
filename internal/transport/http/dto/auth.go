package dto

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type VerifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

type LoginRequest struct {
	Email        string `json:"email" binding:"required,email"`
	Password     string `json:"password" binding:"required"`
	TOTPCode     string `json:"totp_code"`
	RecoveryCode string `json:"recovery_code"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

type UpdateProfileRequest struct {
	FirstName         string `json:"first_name"`
	LastName          string `json:"last_name"`
	AvatarURL         string `json:"avatar_url"`
	CompanyName       string `json:"company_name"`
	PhoneNumber       string `json:"phone_number"`
	Timezone          string `json:"timezone"`
	PreferredLanguage string `json:"preferred_language"`
}

type Enable2FARequest struct {
	Password string `json:"password" binding:"required"`
}

type Confirm2FARequest struct {
	Code string `json:"code" binding:"required"`
}

type Disable2FARequest struct {
	Password     string `json:"password" binding:"required"`
	TOTPCode     string `json:"totp_code"`
	RecoveryCode string `json:"recovery_code"`
}

type DeleteAccountRequest struct {
	Password        string `json:"password" binding:"required"`
	GracePeriodDays int    `json:"grace_period_days"`
}
