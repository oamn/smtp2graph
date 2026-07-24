package main

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

type tokenCredentialFunc func(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error)

func (f tokenCredentialFunc) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return f(ctx, opts)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSendRawMimeMail(t *testing.T) {
	mimeMessage := []byte("Subject: Test\r\n\r\nHello")
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", req.Method)
			}
			if !strings.Contains(req.URL.EscapedPath(), "sender%2Falias@example.com") {
				t.Errorf("escaped path = %q, want escaped user ID", req.URL.EscapedPath())
			}
			if got := req.Header.Get("Authorization"); got != "Bearer access-token" {
				t.Errorf("Authorization = %q, want bearer token", got)
			}
			if got := req.Header.Get("Content-Type"); got != "text/plain" {
				t.Errorf("Content-Type = %q, want text/plain", got)
			}

			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll(request body) error: %v", err)
			}
			decoded, err := base64.StdEncoding.DecodeString(string(body))
			if err != nil {
				t.Fatalf("DecodeString(request body) error: %v", err)
			}
			if string(decoded) != string(mimeMessage) {
				t.Errorf("decoded body = %q, want %q", decoded, mimeMessage)
			}

			return graphResponse(req, http.StatusAccepted, ""), nil
		}),
	}

	err := sendRawMimeMail(context.Background(), client, "access-token", "sender/alias@example.com", mimeMessage)
	if err != nil {
		t.Fatalf("sendRawMimeMail() error: %v", err)
	}
}

func TestSendRawMimeMailDoesNotExposeResponseBody(t *testing.T) {
	const sensitiveBody = `{"error":{"message":"sensitive upstream detail"}}`
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return graphResponse(req, http.StatusForbidden, sensitiveBody), nil
		}),
	}

	err := sendRawMimeMail(context.Background(), client, "access-token", "sender@example.com", []byte("message"))
	if err == nil {
		t.Fatal("sendRawMimeMail() error = nil, want Graph error")
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Errorf("error = %q, want HTTP status", err)
	}
	if strings.Contains(err.Error(), sensitiveBody) || strings.Contains(err.Error(), "sensitive upstream detail") {
		t.Errorf("error exposes Graph response body: %q", err)
	}
}

func TestSendRawMimeMailHonorsClientTimeout(t *testing.T) {
	client := &http.Client{
		Timeout: 20 * time.Millisecond,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}

	start := time.Now()
	err := sendRawMimeMail(context.Background(), client, "access-token", "sender@example.com", []byte("message"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("sendRawMimeMail() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("sendRawMimeMail() took %s, want bounded request", elapsed)
	}
}

func TestGraphMailHandlerBoundsTokenAcquisition(t *testing.T) {
	msg, err := mail.ReadMessage(strings.NewReader("Subject: Test\r\n\r\nHello"))
	if err != nil {
		t.Fatalf("ReadMessage() error: %v", err)
	}

	handler := &graphMailHandler{
		config: &appConfig{SenderEmail: "sender@example.com"},
		cred: tokenCredentialFunc(func(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
			<-ctx.Done()
			return azcore.AccessToken{}, ctx.Err()
		}),
		client:  &http.Client{Timeout: time.Second},
		timeout: 20 * time.Millisecond,
	}

	start := time.Now()
	err = handler.handleMessage(context.Background(), msg)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("handleMessage() error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("handleMessage() took %s, want bounded token acquisition", elapsed)
	}
}

func graphResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Status:     http.StatusText(statusCode),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
