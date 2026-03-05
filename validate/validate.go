package validate

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/shiliu-ai/go-atlas/errors"
	"github.com/shiliu-ai/go-atlas/response"
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
		msg := formatError(err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// BindJSON is a shorthand for JSON body binding.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		msg := formatError(err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// BindQuery is a shorthand for query parameter binding.
func BindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		msg := formatError(err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// BindURI is a shorthand for URI parameter binding.
func BindURI(c *gin.Context, dst any) bool {
	if err := c.ShouldBindUri(dst); err != nil {
		msg := formatError(err)
		response.Fail(c, errors.CodeBadRequest, msg)
		c.Abort()
		return false
	}
	return true
}

// formatError converts validation errors into a human-readable message.
func formatError(err error) string {
	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		msgs := make([]string, 0, len(ve))
		for _, fe := range ve {
			msgs = append(msgs, fieldErrorMsg(fe))
		}
		return strings.Join(msgs, "; ")
	}
	return err.Error()
}

func fieldErrorMsg(fe validator.FieldError) string {
	field := fe.Field()
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", field)
	case "email":
		return fmt.Sprintf("%s must be a valid email", field)
	case "min":
		return fmt.Sprintf("%s must be at least %s", field, fe.Param())
	case "max":
		return fmt.Sprintf("%s must be at most %s", field, fe.Param())
	case "len":
		return fmt.Sprintf("%s must be exactly %s characters", field, fe.Param())
	case "oneof":
		return fmt.Sprintf("%s must be one of [%s]", field, fe.Param())
	case "gt":
		return fmt.Sprintf("%s must be greater than %s", field, fe.Param())
	case "gte":
		return fmt.Sprintf("%s must be greater than or equal to %s", field, fe.Param())
	case "lt":
		return fmt.Sprintf("%s must be less than %s", field, fe.Param())
	case "lte":
		return fmt.Sprintf("%s must be less than or equal to %s", field, fe.Param())
	case "url":
		return fmt.Sprintf("%s must be a valid URL", field)
	default:
		return fmt.Sprintf("%s failed on %s validation", field, fe.Tag())
	}
}
