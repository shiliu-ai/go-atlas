package sms

import "context"

// NewTencent creates a new Tencent Cloud SMS client.
// Stub — replaced in Task 3.
func NewTencent(cfg TencentConfig) (*TencentSMS, error) {
	return nil, nil
}

// TencentSMS implements SMS using Tencent Cloud.
type TencentSMS struct{}

func (t *TencentSMS) Send(_ context.Context, _ *SendRequest) error { return nil }
func (t *TencentSMS) Ping(_ context.Context) error                 { return nil }
