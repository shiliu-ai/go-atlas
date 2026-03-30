package sms

import (
	"context"
	"errors"
)

// Standard SMS errors for unified error handling across providers.
var (
	ErrInvalidPhone  = errors.New("sms: invalid phone number")
	ErrSendFailed    = errors.New("sms: send failed")
	ErrProviderError = errors.New("sms: provider error")
)

// SendRequest contains the parameters for sending an SMS.
type SendRequest struct {
	Phone      string   // E.164 format, e.g. "+8613800138000"
	TemplateID string   // SMS template ID
	Params     []string // Ordered template parameters (e.g. ["1234", "5"])
	Sign       string   // Optional; overrides default sign from config if non-empty
}

// SMS defines the SMS sending capability.
type SMS interface {
	// Send sends an SMS message to a single recipient.
	Send(ctx context.Context, req *SendRequest) error

	// Ping checks provider availability.
	Ping(ctx context.Context) error
}
