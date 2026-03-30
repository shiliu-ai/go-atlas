package validate

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/shiliu-ai/go-atlas/aether/errors"
	"github.com/shiliu-ai/go-atlas/aether/i18n"
	"github.com/shiliu-ai/go-atlas/aether/response"
)

// Bind binds and validates the request payload into dst.
// On validation failure it writes a 400 response with field-level details and returns false.
// Usage:
//
//	var req CreateUserReq
//	if !validate.Bind(c, &req) {
//	    return
//	}
func Bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBind(dst); err != nil {
		msg := formatError(c, err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// BindJSON is a shorthand for JSON body binding.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		msg := formatError(c, err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// BindQuery is a shorthand for query parameter binding.
func BindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		msg := formatError(c, err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// BindURI is a shorthand for URI parameter binding.
func BindURI(c *gin.Context, dst any) bool {
	if err := c.ShouldBindUri(dst); err != nil {
		msg := formatError(c, err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// formatError converts validation errors into a human-readable message.
func formatError(c *gin.Context, err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		msgs := make([]string, 0, len(ve))
		for _, fe := range ve {
			msgs = append(msgs, fieldErrorMsg(c, fe))
		}
		return strings.Join(msgs, "; ")
	}
	return err.Error()
}

func fieldErrorMsg(c *gin.Context, fe validator.FieldError) string {
	ctx := c.Request.Context()
	field := fe.Field()

	switch fe.Tag() {
	case "required":
		return i18n.T(ctx, i18n.MsgValidateRequired, field)
	case "email":
		return i18n.T(ctx, i18n.MsgValidateEmail, field)
	case "min":
		return i18n.T(ctx, i18n.MsgValidateMin, field, fe.Param())
	case "max":
		return i18n.T(ctx, i18n.MsgValidateMax, field, fe.Param())
	case "len":
		return i18n.T(ctx, i18n.MsgValidateLen, field, fe.Param())
	case "oneof":
		return i18n.T(ctx, i18n.MsgValidateOneOf, field, fe.Param())
	case "gt":
		return i18n.T(ctx, i18n.MsgValidateGT, field, fe.Param())
	case "gte":
		return i18n.T(ctx, i18n.MsgValidateGTE, field, fe.Param())
	case "lt":
		return i18n.T(ctx, i18n.MsgValidateLT, field, fe.Param())
	case "lte":
		return i18n.T(ctx, i18n.MsgValidateLTE, field, fe.Param())
	case "url":
		return i18n.T(ctx, i18n.MsgValidateURL, field)
	default:
		return i18n.T(ctx, i18n.MsgValidateDefault, field, fe.Tag())
	}
}
