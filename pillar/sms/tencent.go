package sms

import (
	"context"
	"fmt"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcsms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
)

// TencentSMS implements SMS using Tencent Cloud SMS service.
type TencentSMS struct {
	client *tcsms.Client
	cfg    TencentConfig
}

// NewTencent creates a new Tencent Cloud SMS client.
func NewTencent(cfg TencentConfig) (*TencentSMS, error) {
	if cfg.Region == "" {
		cfg.Region = "ap-guangzhou"
	}

	credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	cpf := profile.NewClientProfile()

	client, err := tcsms.NewClient(credential, cfg.Region, cpf)
	if err != nil {
		return nil, fmt.Errorf("sms: create tencent client: %w", err)
	}

	return &TencentSMS{client: client, cfg: cfg}, nil
}

func (t *TencentSMS) Send(ctx context.Context, req *SendRequest) error {
	if req.Phone == "" {
		return ErrInvalidPhone
	}

	sign := req.Sign
	if sign == "" {
		sign = t.cfg.Sign
	}

	templateID := req.TemplateID
	if templateID == "" {
		templateID = t.cfg.TemplateID
	}

	request := tcsms.NewSendSmsRequest()
	request.SmsSdkAppId = common.StringPtr(t.cfg.AppID)
	request.SignName = common.StringPtr(sign)
	request.TemplateId = common.StringPtr(templateID)
	request.PhoneNumberSet = common.StringPtrs([]string{req.Phone})
	if len(req.Params) > 0 {
		request.TemplateParamSet = common.StringPtrs(req.Params)
	}

	response, err := t.client.SendSmsWithContext(ctx, request)
	if err != nil {
		if sdkErr, ok := err.(*tcerrors.TencentCloudSDKError); ok {
			return fmt.Errorf("%w: [%s] %s", ErrProviderError, sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	// Check per-number send status.
	if response.Response != nil && len(response.Response.SendStatusSet) > 0 {
		status := response.Response.SendStatusSet[0]
		if status.Code != nil && *status.Code != "Ok" {
			msg := ""
			if status.Message != nil {
				msg = *status.Message
			}
			return fmt.Errorf("%w: [%s] %s", ErrSendFailed, *status.Code, msg)
		}
	}

	return nil
}

func (t *TencentSMS) Ping(_ context.Context) error {
	return nil
}
