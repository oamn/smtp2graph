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

// Handler defines the interface for processing SMTP messages.
type Handler interface {
	message(ctx context.Context, msg *mail.Message) error
}

// Session manages SMTP session state and implements SMTP command handlers.
type Session struct {
	config  *Config
	ctx     context.Context
	handler Handler

	auth       bool
	sender     *mail.Address
	recipients []mail.Address
}

// AuthMechanisms returns the supported authentication mechanisms. Only PLAIN is supported.
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *Session) Auth(mech string) (sasl.Server, error) {
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

func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
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

func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
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

func (s *Session) Data(r io.Reader) error {
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
		ReportError(s.ctx, err)
		return err
	}
	// Parse the message as a MIME message.
	msg, err := mail.ReadMessage(bytes.NewReader(b))
	if err != nil {
		// Fallback: treat as plain text, wrap in minimal MIME message.
		from := s.sender.String()
		toList := make([]string, len(s.recipients))
		for i, rcpt := range s.recipients {
			toList[i] = rcpt.String()
		}
		to := strings.Join(toList, ", ")
		// Compose minimal MIME message
		var buf bytes.Buffer
		buf.WriteString("From: " + from + "\r\n")
		buf.WriteString("To: " + to + "\r\n")
		buf.WriteString("Subject: (no subject)\r\n")
		buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		buf.WriteString("\r\n")
		buf.Write(b)
		msg, err = mail.ReadMessage(&buf)
		if err != nil {
			smtpErr := newSMTPError(s.ctx, 550, smtp.EnhancedCode{5, 6, 0}, "invalid message format")
			return smtpErr
		}
	}

	// Build a set of all recipients found in To, Cc, and Bcc headers.
	recipientSet := make(map[string]struct{})
	for _, header := range []string{"To", "Cc", "Bcc"} {
		addrs, err := msg.Header.AddressList(header)
		if err == nil {
			for _, addr := range addrs {
				recipientSet[addr.Address] = struct{}{}
			}
		}
	}
	// Find recipients given to RCPT TO but missing from headers.
	missingRecipients := []string{}
	for _, rcpt := range s.recipients {
		if _, found := recipientSet[rcpt.Address]; !found {
			missingRecipients = append(missingRecipients, rcpt.String())
		}
	}
	if len(missingRecipients) > 0 {
		oldBcc := msg.Header.Get("Bcc")
		// Join missing recipients as a comma-separated string.
		missingStr := strings.Join(missingRecipients, ", ")
		if oldBcc != "" {
			// Append missing recipients to the existing Bcc header.
			msg.Header["Bcc"] = []string{oldBcc + ", " + missingStr}
		} else {
			// Set Bcc header if it was missing.
			msg.Header["Bcc"] = []string{missingStr}
		}
	}

	// Ensure the sender (MAIL FROM) is present in the From header.
	if s.sender != nil {
		fromAddrs, err := msg.Header.AddressList("From")
		found := false
		if err == nil {
			for _, addr := range fromAddrs {
				if addr.Address == s.sender.Address {
					found = true
					break
				}
			}
		}
		if !found {
			// Patch From header if sender is missing.
			msg.Header["From"] = []string{s.sender.String()}
		}
	}

	err = s.handler.message(s.ctx, msg)
	if err != nil {
		smtpErr := newSMTPError(s.ctx, 554, smtp.EnhancedCode{5, 3, 0}, err.Error())
		return smtpErr
	}

	return nil
}

func (s *Session) Reset() {
	s.sender = nil
	s.recipients = nil
}

func (s *Session) Logout() error {
	return nil
}

// newSMTPError creates a new smtp.SMTPError with the given code, enhanced code, and message, and reports it to Sentry.
func newSMTPError(ctx context.Context, code int, enhanced smtp.EnhancedCode, message string) *smtp.SMTPError {
	err := &smtp.SMTPError{
		Code:         code,
		EnhancedCode: enhanced,
		Message:      message,
	}
	ReportError(ctx, err)
	return err
}
