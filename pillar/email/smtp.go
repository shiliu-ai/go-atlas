package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

// smtpClient implements Email using SMTP.
type smtpClient struct {
	cfg SMTPConfig
}

// NewSMTP creates a new SMTP email client.
func NewSMTP(cfg SMTPConfig) (Email, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("email/smtp: host is required")
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("email/smtp: from is required")
	}
	if cfg.Port == 0 {
		cfg.Port = 587
	}
	return &smtpClient{cfg: cfg}, nil
}

func (s *smtpClient) Send(_ context.Context, req *SendRequest) error {
	if req.TemplateID != "" {
		return ErrTemplateNotSupported
	}
	if len(req.To) == 0 {
		return ErrInvalidRecipient
	}
	if req.Subject == "" {
		return fmt.Errorf("email/smtp: subject is required for direct mode")
	}

	from := s.cfg.From
	if req.From != "" {
		from = req.From
	}

	msg := s.buildMessage(from, req)
	recipients := s.collectRecipients(req)
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	if s.cfg.TLS {
		return s.sendWithTLS(addr, auth, from, recipients, msg)
	}
	return smtp.SendMail(addr, auth, from, recipients, msg)
}

func (s *smtpClient) Ping(ctx context.Context) error {
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("email/smtp: ping: %w", err)
	}
	return conn.Close()
}

// sendWithTLS sends email over a direct TLS connection (port 465).
func (s *smtpClient) sendWithTLS(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	tlsConfig := &tls.Config{ServerName: s.cfg.Host}
	conn, err := tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("email/smtp: tls dial: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("email/smtp: new client: %w", err)
	}
	defer client.Close()

	if err = client.Auth(auth); err != nil {
		return fmt.Errorf("email/smtp: auth: %w", err)
	}
	if err = client.Mail(from); err != nil {
		return fmt.Errorf("email/smtp: mail from: %w", err)
	}
	for _, rcpt := range to {
		if err = client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("email/smtp: rcpt %q: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("email/smtp: data: %w", err)
	}
	if _, err = w.Write(msg); err != nil {
		return fmt.Errorf("email/smtp: write: %w", err)
	}
	if err = w.Close(); err != nil {
		return fmt.Errorf("email/smtp: close data: %w", err)
	}

	return client.Quit()
}

// collectRecipients merges To, Cc, and Bcc into a single slice.
func (s *smtpClient) collectRecipients(req *SendRequest) []string {
	recipients := make([]string, 0, len(req.To)+len(req.Cc)+len(req.Bcc))
	recipients = append(recipients, req.To...)
	recipients = append(recipients, req.Cc...)
	recipients = append(recipients, req.Bcc...)
	return recipients
}

// encodeHeader encodes a header value using RFC 2047 if it contains non-ASCII characters.
// QEncoding returns the value unchanged for pure ASCII strings.
func encodeHeader(value string) string {
	return mime.QEncoding.Encode("UTF-8", value)
}

// buildMessage constructs the full MIME message.
func (s *smtpClient) buildMessage(from string, req *SendRequest) []byte {
	var buf bytes.Buffer

	// Headers
	fmt.Fprintf(&buf, "From: %s\r\n", from)
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(req.To, ", "))
	if len(req.Cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(req.Cc, ", "))
	}
	if req.ReplyTo != "" {
		fmt.Fprintf(&buf, "Reply-To: %s\r\n", req.ReplyTo)
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", encodeHeader(req.Subject))
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&buf, "Message-ID: <%d@%s>\r\n", time.Now().UnixNano(), s.cfg.Host)
	buf.WriteString("MIME-Version: 1.0\r\n")

	contentType := string(req.ContentType)
	if contentType == "" {
		contentType = string(ContentTypeHTML)
	}

	if len(req.Attachments) == 0 {
		// Simple message without attachments.
		fmt.Fprintf(&buf, "Content-Type: %s; charset=UTF-8\r\n", contentType)
		buf.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		buf.WriteString(base64.StdEncoding.EncodeToString([]byte(req.Body)))
	} else {
		// Multipart message with attachments.
		writer := multipart.NewWriter(&buf)
		fmt.Fprintf(&buf, "Content-Type: multipart/mixed; boundary=%s\r\n\r\n", writer.Boundary())

		// Body part
		bodyHeader := make(textproto.MIMEHeader)
		bodyHeader.Set("Content-Type", fmt.Sprintf("%s; charset=UTF-8", contentType))
		bodyHeader.Set("Content-Transfer-Encoding", "base64")
		bodyPart, _ := writer.CreatePart(bodyHeader)
		bodyPart.Write([]byte(base64.StdEncoding.EncodeToString([]byte(req.Body))))

		// Attachment parts
		for _, att := range req.Attachments {
			attHeader := make(textproto.MIMEHeader)
			mimeType := att.MIMEType
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			attHeader.Set("Content-Type", mimeType)
			attHeader.Set("Content-Transfer-Encoding", "base64")
			attHeader.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Filename))
			attPart, _ := writer.CreatePart(attHeader)
			attPart.Write([]byte(base64.StdEncoding.EncodeToString(att.Content)))
		}

		writer.Close()
	}

	return buf.Bytes()
}
