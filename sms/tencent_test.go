package sms

import (
	"testing"
)

func TestNewUnsupportedDriver(t *testing.T) {
	_, err := New(Config{Driver: "unknown"})
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
}

func TestManagerGetUnknownInstance(t *testing.T) {
	m := NewManager(map[string]Config{})
	_, err := m.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown instance")
	}
}

func TestManagerNames(t *testing.T) {
	m := NewManager(map[string]Config{
		"default": {Driver: "tencentcloud"},
		"oem":     {Driver: "tencentcloud"},
	})
	names := m.Names()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestNewTencentDefaultRegion(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sms.cfg.Region != "ap-guangzhou" {
		t.Errorf("expected default region ap-guangzhou, got %s", sms.cfg.Region)
	}
}

func TestNewTencentCustomRegion(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
		Region:    "ap-beijing",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sms.cfg.Region != "ap-beijing" {
		t.Errorf("expected region ap-beijing, got %s", sms.cfg.Region)
	}
}

func TestSendInvalidPhone(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = sms.Send(t.Context(), &SendRequest{
		Phone:      "",
		TemplateID: "123",
		Params:     []string{"1234"},
	})
	if err != ErrInvalidPhone {
		t.Errorf("expected ErrInvalidPhone, got %v", err)
	}
}

func TestPingReturnsNil(t *testing.T) {
	sms, err := NewTencent(TencentConfig{
		SecretID:  "test-id",
		SecretKey: "test-key",
		AppID:     "1400000000",
		Sign:      "TestSign",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := sms.Ping(t.Context()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
