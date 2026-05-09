package response

import (
	"net/http"

	apperrors "firmflow/internal/common/errors"

	"github.com/gin-gonic/gin"
)

type SuccessEnvelope struct {
	Data interface{} `json:"data"`
	Meta interface{} `json:"meta,omitempty"`
}

type ErrorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

type ErrorPayload struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"request_id"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, SuccessEnvelope{Data: data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, SuccessEnvelope{Data: data})
}

func WithMeta(c *gin.Context, status int, data, meta interface{}) {
	c.JSON(status, SuccessEnvelope{Data: data, Meta: meta})
}

func Fail(c *gin.Context, err error) {
	reqID, _ := c.Get("request_id")

	apiErr, ok := err.(apperrors.AppError)
	if !ok {
		apiErr = apperrors.Internal()
	}

	c.JSON(apiErr.HTTPStatus, ErrorEnvelope{
		Error: ErrorPayload{
			Code:      apiErr.Code,
			Message:   apiErr.Message,
			Details:   apiErr.Details,
			RequestID: toString(reqID),
		},
	})
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}
