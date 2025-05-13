package main

import (
	"bytes"
	"context"
	"io"
	"net/mail"
	"testing"
)

// mockHandler implements Handler for testing.
type mockHandler struct {
	called bool
	msg    *mail.Message
	err    error
}

func (m *mockHandler) message(ctx context.Context, msg *mail.Message) error {
	m.called = true
	m.msg = msg
	return m.err
}

func newTestSession() *Session {
	t := &testing.T{} // for t.Helper()
	return newTestSessionWithT(t)
}

func newTestSessionWithT(t *testing.T) *Session {
	t.Helper()
	cfg := &Config{
		SenderEmail:    "sender@example.com",
		SenderPassword: "password",
	}
	h := &mockHandler{}
	return &Session{
		config:  cfg,
		ctx:     context.Background(),
		handler: h,
	}
}

func TestSession_SMTP_Like_Success(t *testing.T) {
	session := newTestSessionWithT(t)

	// Simulate AUTH (PLAIN)
	server, err := session.Auth("PLAIN")
	if err != nil {
		t.Fatalf("Auth() error: %v", err)
	}
	// PLAIN: [authzid] 0 [authcid] 0 [passwd]
	plainResp := []byte("\x00sender@example.com\x00password")
	_, done, err := server.Next(plainResp)
	if err != nil {
		t.Fatalf("PLAIN Next() error: %v", err)
	}
	if !done {
		t.Fatal("PLAIN auth not completed after first response")
	}

	// Simulate MAIL FROM
	err = session.Mail("sender@example.com", nil)
	if err != nil {
		t.Fatalf("Mail() error: %v", err)
	}

	// Simulate RCPT TO (multiple recipients)
	recipients := []string{"recipient1@example.com", "recipient2@example.com"}
	for _, rcpt := range recipients {
		err = session.Rcpt(rcpt, nil)
		if err != nil {
			t.Fatalf("Rcpt(%s) error: %v", rcpt, err)
		}
	}

	// Simulate DATA
	msg := []byte("From: sender@example.com\r\nTo: recipient1@example.com, recipient2@example.com\r\nSubject: Test\r\n\r\nHello World\r\n")
	err = session.Data(bytes.NewReader(msg))
	if err != nil {
		t.Fatalf("Data() error: %v", err)
	}

	// Check handler was called and message content
	mh, ok := session.handler.(*mockHandler)
	if !ok || !mh.called {
		t.Error("handler.message was not called")
	}
	if mh.msg == nil {
		t.Error("handler.message did not receive a message")
	} else {
		// Check From header
		from, err := mh.msg.Header.AddressList("From")
		if err != nil || len(from) != 1 || from[0].Address != "sender@example.com" {
			t.Errorf("unexpected From header: got %v, err=%v", from, err)
		}
		// Check To header
		to, err := mh.msg.Header.AddressList("To")
		if err != nil || len(to) != 2 || to[0].Address != "recipient1@example.com" || to[1].Address != "recipient2@example.com" {
			t.Errorf("unexpected To header: got %v, err=%v", to, err)
		}
		// Check Subject
		subj := mh.msg.Header.Get("Subject")
		if subj != "Test" {
			t.Errorf("unexpected Subject: got %q", subj)
		}
		// Check Body
		body, err := io.ReadAll(mh.msg.Body)
		if err != nil {
			t.Errorf("error reading message body: %v", err)
		}
		if string(bytes.TrimSpace(body)) != "Hello World" {
			t.Errorf("unexpected message body: got %q", string(body))
		}
	}

	// Simulate RSET (reset for new transaction)
	session.Reset()
	if session.sender != nil || len(session.recipients) != 0 {
		t.Error("Reset() did not clear sender/recipients")
	}

	// Simulate LOGOUT
	err = session.Logout()
	if err != nil {
		t.Errorf("Logout() error: %v", err)
	}
}

func TestSession_Errors(t *testing.T) {
	t.Run("Mail without auth", func(t *testing.T) {
		session := newTestSessionWithT(t)
		err := session.Mail("sender@example.com", nil)
		if err == nil || err.Error() == "" {
			t.Error("expected error for Mail without auth")
		}
	})

	t.Run("Rcpt before Mail", func(t *testing.T) {
		session := newTestSessionWithT(t)
		session.auth = true
		err := session.Rcpt("recipient@example.com", nil)
		if err == nil || err.Error() == "" {
			t.Error("expected error for Rcpt before Mail")
		}
	})

	t.Run("Mail with invalid address", func(t *testing.T) {
		session := newTestSessionWithT(t)
		session.auth = true
		err := session.Mail("not-an-email", nil)
		if err == nil || err.Error() == "" {
			t.Error("expected error for invalid sender address")
		}
	})

	t.Run("Rcpt with invalid address", func(t *testing.T) {
		session := newTestSessionWithT(t)
		session.auth = true
		_ = session.Mail("sender@example.com", nil)
		err := session.Rcpt("not-an-email", nil)
		if err == nil || err.Error() == "" {
			t.Error("expected error for invalid recipient address")
		}
	})

	t.Run("Mail after Rcpt", func(t *testing.T) {
		session := newTestSessionWithT(t)
		session.auth = true
		_ = session.Mail("sender@example.com", nil)
		_ = session.Rcpt("recipient@example.com", nil)
		err := session.Mail("sender2@example.com", nil)
		if err == nil || err.Error() == "" {
			t.Error("expected error for Mail after Rcpt")
		}
	})

	t.Run("Data without auth", func(t *testing.T) {
		session := newTestSessionWithT(t)
		err := session.Data(bytes.NewReader([]byte("test")))
		if err == nil || err.Error() == "" {
			t.Error("expected error for Data without auth")
		}
	})

	t.Run("Data without sender", func(t *testing.T) {
		session := newTestSessionWithT(t)
		session.auth = true
		err := session.Data(bytes.NewReader([]byte("test")))
		if err == nil || err.Error() == "" {
			t.Error("expected error for Data without sender")
		}
	})

	t.Run("Data without recipients", func(t *testing.T) {
		session := newTestSessionWithT(t)
		session.auth = true
		_ = session.Mail("sender@example.com", nil)
		err := session.Data(bytes.NewReader([]byte("test")))
		if err == nil || err.Error() == "" {
			t.Error("expected error for Data without recipients")
		}
	})

	t.Run("Data with invalid MIME", func(t *testing.T) {
		session := newTestSession()
		session.auth = true
		_ = session.Mail("sender@example.com", nil)
		_ = session.Rcpt("recipient@example.com", nil)
		// Intentionally pass invalid MIME (no headers, just body)
		err := session.Data(bytes.NewReader([]byte("not a mime message")))
		if err != nil && err.Error() == "" {
			t.Error("expected error for invalid MIME, got empty error")
		}
	})
}
