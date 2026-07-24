package main

import (
	"context"
	"errors"
	"net"
	"net/mail"
	"os"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

type blockingMessageHandler struct {
	started chan struct{}
	release chan struct{}
}

func (h *blockingMessageHandler) handleMessage(ctx context.Context, msg *mail.Message) error {
	close(h.started)
	select {
	case <-h.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestServeSMTPServerDrainsActiveMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handler := &blockingMessageHandler{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	cfg := testServerConfig()
	server := newSMTPServer(&smtpBackend{config: cfg, ctx: ctx, handler: handler}, cfg)
	listener := testListener(t)
	shutdownCh := make(chan os.Signal, 1)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveSMTPServer(server, listener, shutdownCh, cancel, time.Second)
	}()

	sendDone := make(chan error, 1)
	go func() {
		sendDone <- sendTestMessage(listener.Addr().String(), cfg)
	}()

	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("message handler did not start")
	}

	shutdownCh <- os.Interrupt
	select {
	case <-ctx.Done():
		t.Fatal("server canceled active message before the drain deadline")
	case <-time.After(50 * time.Millisecond):
	}

	close(handler.release)
	if err := waitForError(t, sendDone); err != nil {
		t.Fatalf("send message error: %v", err)
	}
	if err := waitForError(t, serveDone); err != nil {
		t.Fatalf("serveSMTPServer() error: %v", err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("server context was not canceled after draining")
	}
}

func TestServeSMTPServerBoundsShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := testServerConfig()
	cfg.ReadTimeout = 0
	handler := &blockingMessageHandler{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	server := newSMTPServer(&smtpBackend{
		config:  cfg,
		ctx:     ctx,
		handler: handler,
	}, cfg)
	listener := testListener(t)
	shutdownCh := make(chan os.Signal, 1)
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- serveSMTPServer(server, listener, shutdownCh, cancel, 20*time.Millisecond)
	}()

	sendDone := make(chan error, 1)
	go func() {
		sendDone <- sendTestMessage(listener.Addr().String(), cfg)
	}()
	select {
	case <-handler.started:
	case <-time.After(time.Second):
		t.Fatal("message handler did not start")
	}

	start := time.Now()
	shutdownCh <- os.Interrupt
	err := waitForError(t, serveDone)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("serveSMTPServer() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("serveSMTPServer() took %s, want bounded shutdown", elapsed)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("server context was not canceled after shutdown timeout")
	}
	if err := waitForError(t, sendDone); err == nil {
		t.Fatal("send message error = nil, want canceled delivery")
	}
}

func TestRunReturnsConfigFailure(t *testing.T) {
	for _, key := range []string{
		"ENTRA_CLIENT_ID",
		"ENTRA_TENANT_ID",
		"ENTRA_CLIENT_SECRET",
		"SENDER_EMAIL",
		"SENDER_PASSWORD",
	} {
		t.Setenv(key, "")
	}

	if exitCode := run(nil); exitCode != 1 {
		t.Fatalf("run() exit code = %d, want 1", exitCode)
	}
}

func TestRunReturnsSuccessForHelp(t *testing.T) {
	if exitCode := run([]string{"-help"}); exitCode != 0 {
		t.Fatalf("run() exit code = %d, want 0", exitCode)
	}
}

func testServerConfig() *appConfig {
	return &appConfig{
		SMTPAddr:        "127.0.0.1:0",
		SMTPDomain:      "localhost",
		MaxMessageBytes: maxGraphCompatibleMessageBytes,
		MaxRecipients:   50,
		WriteTimeout:    time.Second,
		ReadTimeout:     time.Second,
		SenderEmail:     "sender@example.com",
		SenderPassword:  "password",
	}
}

func testListener(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
	})
	return listener
}

func sendTestMessage(addr string, cfg *appConfig) error {
	client, err := smtp.Dial(addr)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Auth(sasl.NewPlainClient("", cfg.SenderEmail, cfg.SenderPassword)); err != nil {
		return err
	}
	if err := client.Mail(cfg.SenderEmail, nil); err != nil {
		return err
	}
	if err := client.Rcpt("recipient@example.com", nil); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write([]byte("Subject: Test\r\n\r\nHello")); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func waitForError(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for operation")
		return nil
	}
}
