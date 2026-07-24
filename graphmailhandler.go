// Package main provides a handler for sending emails using Microsoft Graph API.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	policy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	azidentity "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

const graphRequestTimeout = 30 * time.Second

// graphMailHandler implements the messageHandler interface and relays messages to Microsoft Graph API.
type graphMailHandler struct {
	config  *appConfig
	cred    azcore.TokenCredential
	client  *http.Client
	timeout time.Duration
}

// newGraphMailHandler creates a new graphMailHandler with a single ClientSecretCredential instance.
func newGraphMailHandler(config *appConfig) (*graphMailHandler, error) {
	cred, err := azidentity.NewClientSecretCredential(
		config.EntraTenantID,
		config.EntraClientID,
		config.EntraClientSecret,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &graphMailHandler{
		config:  config,
		cred:    cred,
		client:  &http.Client{Timeout: graphRequestTimeout},
		timeout: graphRequestTimeout,
	}, nil
}

// handleMessage relays the given MIME message to Microsoft Graph API.
func (h *graphMailHandler) handleMessage(ctx context.Context, msg *mail.Message) error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	mimeMessage, err := encodeMailMessage(msg)
	if err != nil {
		return fmt.Errorf("encodeMailMessage: %w", err)
	}

	accessToken, err := h.cred.GetToken(ctx, policy.TokenRequestOptions{
		Scopes: []string{"https://graph.microsoft.com/.default"},
	})
	if err != nil {
		return fmt.Errorf("GetToken: %w", err)
	}

	if err := sendRawMimeMail(ctx, h.client, accessToken.Token, h.config.SenderEmail, mimeMessage); err != nil {
		return fmt.Errorf("sendRawMimeMail: %w", err)
	}

	return nil
}

// encodeMailMessage encodes a mail.Message into raw []byte in RFC822 format.
func encodeMailMessage(msg *mail.Message) ([]byte, error) {
	var buf bytes.Buffer
	// Write headers
	for k, v := range msg.Header {
		for _, vv := range v {
			// Write header line: Key: Value\r\n
			if _, err := buf.WriteString(k + ": " + vv + "\r\n"); err != nil {
				return nil, err
			}
		}
	}
	// Blank line between headers and body
	if _, err := buf.WriteString("\r\n"); err != nil {
		return nil, err
	}
	// Write body
	if msg.Body != nil {
		if _, err := buf.ReadFrom(msg.Body); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// sendRawMimeMail posts a base64-encoded MIME message to the Graph API /sendMail endpoint.
// accessToken: a valid OAuth2 token for Microsoft Graph with Mail.Send permission
// userID: the user ID or email address to send as
// mimeMessage: the full RFC 5322 message (headers + body)
// The official Go SDK does not support sending raw MIME messages, so we use a direct HTTP request.
func sendRawMimeMail(ctx context.Context, client *http.Client, accessToken string, userID string, mimeMessage []byte) error {
	endpoint := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", url.PathEscape(userID))
	encoded := base64.StdEncoding.EncodeToString(mimeMessage)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(encoded))
	if err != nil {
		return fmt.Errorf("NewRequestWithContext: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http.Do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64*1024))
		return fmt.Errorf("sendMail failed: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return nil
}
