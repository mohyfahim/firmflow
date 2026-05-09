package handlers

import (
	apperrors "firmflow/internal/common/errors"
	"firmflow/internal/common/response"
	"firmflow/internal/common/validator"
	"firmflow/internal/domain/auth/model"
	"firmflow/internal/domain/auth/service"
	"firmflow/internal/transport/http/dto"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthHandler struct {
	svc *service.Service
}

func NewAuthHandler(svc *service.Service) *AuthHandler { return &AuthHandler{svc: svc} }

func (h *AuthHandler) Register(c *gin.Context) {
	var req dto.RegisterRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.Register(c.Request.Context(), req.Email, req.Password); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "registration successful; verify your email"})
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req dto.VerifyEmailRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "email verified"})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	out, err := h.svc.Login(c.Request.Context(), req.Email, req.Password, req.TOTPCode, req.RecoveryCode, service.SessionMeta{
		UserAgent: c.GetHeader("User-Agent"),
		IP:        c.ClientIP(),
	})
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, out)
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	out, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, service.SessionMeta{
		UserAgent: c.GetHeader("User-Agent"),
		IP:        c.ClientIP(),
	})
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, out)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	_ = h.svc.Logout(c.Request.Context(), req.RefreshToken)
	response.OK(c, gin.H{"message": "logged out"})
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req dto.ForgotPasswordRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	_ = h.svc.ForgotPassword(c.Request.Context(), req.Email)
	response.OK(c, gin.H{"message": "if the email exists, reset instructions were sent"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), req.Token, req.NewPassword); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "password has been reset"})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	userID, sessionID, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), userID, sessionID, req.CurrentPassword, req.NewPassword); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "password changed; other sessions revoked"})
}

func (h *AuthHandler) GetProfile(c *gin.Context) {
	userID, _, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	profile, err := h.svc.GetProfile(c.Request.Context(), userID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, profile)
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	var req dto.UpdateProfileRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	userID, _, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	out, err := h.svc.UpdateProfile(c.Request.Context(), userID, model.UserProfile{
		FirstName:         req.FirstName,
		LastName:          req.LastName,
		AvatarURL:         req.AvatarURL,
		CompanyName:       req.CompanyName,
		PhoneNumber:       req.PhoneNumber,
		Timezone:          req.Timezone,
		PreferredLanguage: req.PreferredLanguage,
	})
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, out)
}

func (h *AuthHandler) ListSessions(c *gin.Context) {
	userID, sessionID, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	out, err := h.svc.ListSessions(c.Request.Context(), userID, sessionID)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, out)
}

func (h *AuthHandler) RevokeSession(c *gin.Context) {
	userID, _, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	id, err := uuid.Parse(c.Param("sessionID"))
	if err != nil {
		c.Error(apperrors.BadRequest("invalid session id", nil))
		return
	}
	if err := h.svc.RevokeSession(c.Request.Context(), userID, id); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "session revoked"})
}

func (h *AuthHandler) RevokeOtherSessions(c *gin.Context) {
	userID, sessionID, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.RevokeOtherSessions(c.Request.Context(), userID, sessionID); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "other sessions revoked"})
}

func (h *AuthHandler) BeginEnable2FA(c *gin.Context) {
	var req dto.Enable2FARequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	userID, _, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	out, err := h.svc.BeginEnable2FA(c.Request.Context(), userID, req.Password)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, out)
}

func (h *AuthHandler) ConfirmEnable2FA(c *gin.Context) {
	var req dto.Confirm2FARequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	userID, _, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	codes, err := h.svc.ConfirmEnable2FA(c.Request.Context(), userID, req.Code)
	if err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"recovery_codes": codes})
}

func (h *AuthHandler) Disable2FA(c *gin.Context) {
	var req dto.Disable2FARequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	userID, _, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.Disable2FA(c.Request.Context(), userID, req.Password, req.TOTPCode, req.RecoveryCode); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "2fa disabled"})
}

func (h *AuthHandler) DeleteAccount(c *gin.Context) {
	var req dto.DeleteAccountRequest
	if err := validator.BindJSON(c, &req); err != nil {
		c.Error(err)
		return
	}
	userID, _, err := authContext(c)
	if err != nil {
		c.Error(err)
		return
	}
	if err := h.svc.DeleteAccount(c.Request.Context(), userID, req.Password, req.GracePeriodDays); err != nil {
		c.Error(err)
		return
	}
	response.OK(c, gin.H{"message": "account deletion scheduled"})
}

func authContext(c *gin.Context) (uuid.UUID, uuid.UUID, error) {
	userRaw := c.GetString("auth_user_id")
	sessionRaw := c.GetString("auth_session_id")
	userID, err := uuid.Parse(userRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, apperrors.Unauthorized()
	}
	sessionID, err := uuid.Parse(sessionRaw)
	if err != nil {
		return uuid.Nil, uuid.Nil, apperrors.Unauthorized()
	}
	return userID, sessionID, nil
}
