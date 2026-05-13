package errors

import "net/http"

type AppError struct {
	Code       string      `json:"code"`
	Message    string      `json:"message"`
	Details    interface{} `json:"details,omitempty"`
	HTTPStatus int         `json:"-"`
}

func (e AppError) Error() string {
	return e.Message
}

func New(code, message string, httpStatus int, details interface{}) AppError {
	return AppError{
		Code:       code,
		Message:    message,
		Details:    details,
		HTTPStatus: httpStatus,
	}
}

func Internal() AppError {
	return New("internal_error", "an unexpected error occurred", http.StatusInternalServerError, nil)
}

func BadRequest(message string, details interface{}) AppError {
	return New("bad_request", message, http.StatusBadRequest, details)
}

func Unauthorized() AppError {
	return New("unauthorized", "authentication is required", http.StatusUnauthorized, nil)
}

func Forbidden() AppError {
	return New("forbidden", "you do not have permission to perform this action", http.StatusForbidden, nil)
}

func NotFound(message string) AppError {
	return New("not_found", message, http.StatusNotFound, nil)
}

func Conflict(code, message string, details interface{}) AppError {
	return New(code, message, http.StatusConflict, details)
}
