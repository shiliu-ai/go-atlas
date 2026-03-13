package i18n

import "golang.org/x/text/language"

// Built-in message keys used by the framework.
const (
	// Response messages.
	MsgOK            = "response.ok"
	MsgInternalError = "response.internal_error"

	// Validation messages.
	MsgValidateRequired = "validate.required"
	MsgValidateEmail    = "validate.email"
	MsgValidateMin      = "validate.min"
	MsgValidateMax      = "validate.max"
	MsgValidateLen      = "validate.len"
	MsgValidateOneOf    = "validate.oneof"
	MsgValidateGT       = "validate.gt"
	MsgValidateGTE      = "validate.gte"
	MsgValidateLT       = "validate.lt"
	MsgValidateLTE      = "validate.lte"
	MsgValidateURL      = "validate.url"
	MsgValidateDefault  = "validate.default"
)

// MessagesEN contains the default English translations for framework messages.
var MessagesEN = map[string]string{
	MsgOK:            "ok",
	MsgInternalError: "internal error",

	MsgValidateRequired: "%s is required",
	MsgValidateEmail:    "%s must be a valid email",
	MsgValidateMin:      "%s must be at least %s",
	MsgValidateMax:      "%s must be at most %s",
	MsgValidateLen:      "%s must be exactly %s characters",
	MsgValidateOneOf:    "%s must be one of [%s]",
	MsgValidateGT:       "%s must be greater than %s",
	MsgValidateGTE:      "%s must be greater than or equal to %s",
	MsgValidateLT:       "%s must be less than %s",
	MsgValidateLTE:      "%s must be less than or equal to %s",
	MsgValidateURL:      "%s must be a valid URL",
	MsgValidateDefault:  "%s failed on %s validation",
}

// MessagesZH contains the Simplified Chinese translations for framework messages.
var MessagesZH = map[string]string{
	MsgOK:            "成功",
	MsgInternalError: "服务器内部错误",

	MsgValidateRequired: "%s 不能为空",
	MsgValidateEmail:    "%s 必须是有效的邮箱地址",
	MsgValidateMin:      "%s 不能小于 %s",
	MsgValidateMax:      "%s 不能大于 %s",
	MsgValidateLen:      "%s 长度必须为 %s",
	MsgValidateOneOf:    "%s 必须是 [%s] 中的一个",
	MsgValidateGT:       "%s 必须大于 %s",
	MsgValidateGTE:      "%s 必须大于或等于 %s",
	MsgValidateLT:       "%s 必须小于 %s",
	MsgValidateLTE:      "%s 必须小于或等于 %s",
	MsgValidateURL:      "%s 必须是有效的 URL",
	MsgValidateDefault:  "%s 的 %s 验证失败",
}

// RegisterDefaults registers the built-in English and Chinese translations
// to the given bundle.
func RegisterDefaults(b *Bundle) {
	b.Register(language.English, MessagesEN)
	b.Register(language.SimplifiedChinese, MessagesZH)
}
