package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/mail"
	"strings"

	"crypto/subtle"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// messageHandler defines the interface for processing SMTP messages.
type messageHandler interface {
	handleMessage(ctx context.Context, msg *mail.Message) error
}

// smtpSession manages SMTP session state and implements SMTP command handlers.
type smtpSession struct {
	config  *appConfig
	ctx     context.Context
	handler messageHandler

	auth       bool
	sender     *mail.Address
	recipients []mail.Address
}

// AuthMechanisms returns the supported authentication mechanisms. Only PLAIN is supported.
func (s *smtpSession) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *smtpSession) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		usernameMatch := subtle.ConstantTimeCompare([]byte(username), []byte(s.config.SenderEmail)) == 1
		passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(s.config.SenderPassword)) == 1
		if !usernameMatch || !passwordMatch {
			return errors.New("invalid username or password")
		}

		s.auth = true
		return nil
	}), nil
}

func (s *smtpSession) Mail(from string, opts *smtp.MailOptions) error {
	if !s.auth {
		err := newSMTPError(s.ctx, 530, smtp.EnhancedCode{5, 7, 0}, "authentication required")
		return err
	}

	// Only allow one sender per SMTP transaction; MAIL FROM must be first.
	if s.sender != nil {
		err := newSMTPError(s.ctx, 503, smtp.EnhancedCode{5, 5, 1}, "sender already specified")
		return err
	}
	if len(s.recipients) > 0 {
		err := newSMTPError(s.ctx, 503, smtp.EnhancedCode{5, 5, 1}, "bad sequence of commands: MAIL FROM after RCPT TO")
		return err
	}

	addr, err := mail.ParseAddress(from)
	if err != nil {
		smtpErr := newSMTPError(s.ctx, 550, smtp.EnhancedCode{5, 1, 7}, "invalid sender address")
		return smtpErr
	}
	s.sender = addr

	return nil
}

func (s *smtpSession) Rcpt(to string, opts *smtp.RcptOptions) error {
	if !s.auth {
		err := newSMTPError(s.ctx, 530, smtp.EnhancedCode{5, 7, 0}, "authentication required")
		return err
	}

	// RCPT TO is not allowed before MAIL FROM.
	if s.sender == nil {
		err := newSMTPError(s.ctx, 503, smtp.EnhancedCode{5, 5, 1}, "bad sequence of commands: RCPT TO before MAIL FROM")
		return err
	}
	// Validate recipient address before accepting.
	addr, err := mail.ParseAddress(to)
	if err != nil {
		smtpErr := newSMTPError(s.ctx, 550, smtp.EnhancedCode{5, 1, 3}, "invalid recipient address")
		return smtpErr
	}

	s.recipients = append(s.recipients, *addr)

	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	if !s.auth {
		err := newSMTPError(s.ctx, 530, smtp.EnhancedCode{5, 7, 0}, "authentication required")
		return err
	}
	if s.sender == nil {
		err := newSMTPError(s.ctx, 503, smtp.EnhancedCode{5, 5, 1}, "sender not specified")
		return err
	}
	if len(s.recipients) == 0 {
		err := newSMTPError(s.ctx, 503, smtp.EnhancedCode{5, 5, 1}, "no recipients specified")
		return err
	}

	b, err := io.ReadAll(r)
	if err != nil {
		reportError(s.ctx, err)
		return err
	}

	msg, err := parseMessage(b, s.sender, s.recipients)
	if err != nil {
		smtpErr := newSMTPError(s.ctx, 550, smtp.EnhancedCode{5, 6, 0}, "invalid message format")
		return smtpErr
	}

	err = s.handler.handleMessage(s.ctx, msg)
	if err != nil {
		smtpErr := newSMTPError(s.ctx, 554, smtp.EnhancedCode{5, 3, 0}, err.Error())
		return smtpErr
	}

	return nil
}

func (s *smtpSession) Reset() {
	s.sender = nil
	s.recipients = nil
}

func (s *smtpSession) Logout() error {
	return nil
}

func parseMessage(raw []byte, sender *mail.Address, recipients []mail.Address) (*mail.Message, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		msg, err = plainTextMessage(raw, sender, recipients)
		if err != nil {
			return nil, err
		}
	}

	normalizeEnvelopeHeaders(msg, sender, recipients)
	return msg, nil
}

func plainTextMessage(raw []byte, sender *mail.Address, recipients []mail.Address) (*mail.Message, error) {
	toList := make([]string, len(recipients))
	for i, rcpt := range recipients {
		toList[i] = rcpt.String()
	}

	var buf bytes.Buffer
	if sender != nil {
		buf.WriteString("From: " + sender.String() + "\r\n")
	}
	buf.WriteString("To: " + strings.Join(toList, ", ") + "\r\n")
	buf.WriteString("Subject: (no subject)\r\n")
	buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	buf.WriteString("\r\n")
	buf.Write(raw)
	return mail.ReadMessage(&buf)
}

func normalizeEnvelopeHeaders(msg *mail.Message, sender *mail.Address, recipients []mail.Address) {
	addMissingRecipientsToBcc(msg, recipients)

	if sender != nil && !headerContainsAddress(msg.Header, "From", sender.Address) {
		msg.Header["From"] = []string{sender.String()}
	}
}

func addMissingRecipientsToBcc(msg *mail.Message, recipients []mail.Address) {
	recipientSet := recipientHeaderSet(msg.Header)

	missingRecipients := make([]string, 0)
	for _, rcpt := range recipients {
		if _, found := recipientSet[rcpt.Address]; !found {
			missingRecipients = append(missingRecipients, rcpt.String())
		}
	}
	if len(missingRecipients) == 0 {
		return
	}

	oldBcc := msg.Header.Get("Bcc")
	missingStr := strings.Join(missingRecipients, ", ")
	if oldBcc != "" {
		msg.Header["Bcc"] = []string{oldBcc + ", " + missingStr}
		return
	}
	msg.Header["Bcc"] = []string{missingStr}
}

func recipientHeaderSet(header mail.Header) map[string]struct{} {
	recipients := make(map[string]struct{})
	for _, field := range []string{"To", "Cc", "Bcc"} {
		addrs, err := header.AddressList(field)
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			recipients[addr.Address] = struct{}{}
		}
	}
	return recipients
}

func headerContainsAddress(header mail.Header, field, address string) bool {
	addrs, err := header.AddressList(field)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		if addr.Address == address {
			return true
		}
	}
	return false
}

// newSMTPError creates a new smtp.SMTPError with the given code, enhanced code, and message, and reports it to Sentry.
func newSMTPError(ctx context.Context, code int, enhanced smtp.EnhancedCode, message string) *smtp.SMTPError {
	err := &smtp.SMTPError{
		Code:         code,
		EnhancedCode: enhanced,
		Message:      message,
	}
	reportError(ctx, err)
	return err
}
