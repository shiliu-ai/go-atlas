package email

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	tcerrors "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	tcses "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/ses/v20201002"
)

// TencentEmail implements Email using Tencent Cloud SES.
type TencentEmail struct {
	client *tcses.Client
	cfg    TencentConfig
}

// NewTencent creates a new Tencent Cloud SES email client.
func NewTencent(cfg TencentConfig) (Email, error) {
	if cfg.Region == "" {
		cfg.Region = "ap-hongkong"
	}

	credential := common.NewCredential(cfg.SecretID, cfg.SecretKey)
	cpf := profile.NewClientProfile()

	client, err := tcses.NewClient(credential, cfg.Region, cpf)
	if err != nil {
		return nil, fmt.Errorf("email: create tencent client: %w", err)
	}

	return &TencentEmail{client: client, cfg: cfg}, nil
}

func (t *TencentEmail) Send(ctx context.Context, req *SendRequest) error {
	if len(req.To) == 0 {
		return ErrInvalidRecipient
	}

	from := t.cfg.From
	if req.From != "" {
		from = req.From
	}

	request := tcses.NewSendEmailRequest()
	request.FromEmailAddress = common.StringPtr(from)
	request.Destination = common.StringPtrs(req.To)

	if len(req.Cc) > 0 {
		request.Cc = common.StringPtrs(req.Cc)
	}
	if len(req.Bcc) > 0 {
		request.Bcc = common.StringPtrs(req.Bcc)
	}

	if req.ReplyTo != "" {
		request.ReplyToAddresses = common.StringPtr(req.ReplyTo)
	}

	templateID := req.TemplateID
	if templateID == "" {
		templateID = t.cfg.TemplateID
	}

	if templateID != "" {
		// Template mode.
		tid, err := strconv.ParseUint(templateID, 10, 64)
		if err != nil {
			return fmt.Errorf("email/tencent: invalid template ID %q: %w", templateID, err)
		}
		templateData, err := json.Marshal(req.TemplateData)
		if err != nil {
			return fmt.Errorf("email/tencent: marshal template data: %w", err)
		}
		request.Template = &tcses.Template{
			TemplateID:   common.Uint64Ptr(tid),
			TemplateData: common.StringPtr(string(templateData)),
		}
		if req.Subject != "" {
			request.Subject = common.StringPtr(req.Subject)
		}
	} else {
		// Direct mode. The Simple.Html and Simple.Text fields require base64-encoded content.
		request.Subject = common.StringPtr(req.Subject)
		if req.ContentType == ContentTypePlain {
			request.Simple = &tcses.Simple{
				Text: common.StringPtr(base64.StdEncoding.EncodeToString([]byte(req.Body))),
			}
		} else {
			request.Simple = &tcses.Simple{
				Html: common.StringPtr(base64.StdEncoding.EncodeToString([]byte(req.Body))),
			}
		}
	}

	// Attachments.
	if len(req.Attachments) > 0 {
		attachments := make([]*tcses.Attachment, 0, len(req.Attachments))
		for _, att := range req.Attachments {
			attachments = append(attachments, &tcses.Attachment{
				FileName: common.StringPtr(att.Filename),
				Content:  common.StringPtr(base64.StdEncoding.EncodeToString(att.Content)),
			})
		}
		request.Attachments = attachments
	}

	_, err := t.client.SendEmailWithContext(ctx, request)
	if err != nil {
		if sdkErr, ok := err.(*tcerrors.TencentCloudSDKError); ok {
			return fmt.Errorf("%w: [%s] %s", ErrProviderError, sdkErr.GetCode(), sdkErr.GetMessage())
		}
		return fmt.Errorf("%w: %v", ErrProviderError, err)
	}

	return nil
}

func (t *TencentEmail) Ping(_ context.Context) error {
	return nil
}
