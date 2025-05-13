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
	"sync"
	"time"

	policy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	azidentity "github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

// GraphMailHandler implements the Handler interface and relays messages to Microsoft Graph API.
type GraphMailHandler struct {
	config *Config
	ctx    context.Context
	cred   *azidentity.ClientSecretCredential

	token      string
	tokenExp   int64 // Unix seconds
	tokenMutex sync.Mutex
}

// NewGraphMailHandler creates a new GraphMailHandler with a single ClientSecretCredential instance.
func NewGraphMailHandler(ctx context.Context, config *Config) (*GraphMailHandler, error) {
	cred, err := azidentity.NewClientSecretCredential(
		config.EntraTenantID,
		config.EntraClientID,
		config.EntraClientSecret,
		nil,
	)
	if err != nil {
		return nil, err
	}

	return &GraphMailHandler{
		config: config,
		ctx:    ctx,
		cred:   cred,
	}, nil
}

// message relays the given MIME message to Microsoft Graph API.
// It parses headers and calls sendMailViaGraph (stub).
func (h *GraphMailHandler) message(ctx context.Context, msg *mail.Message) error {
	mimeMessage, err := encodeMailMessage(msg)
	if err != nil {
		return fmt.Errorf("encodeMailMessage: %w", err)
	}

	accessToken, err := h.getCachedToken()
	if err != nil {
		return fmt.Errorf("getCachedToken: %w", err)
	}

	if err := sendRawMimeMail(ctx, accessToken, h.config.SenderEmail, mimeMessage); err != nil {
		return fmt.Errorf("sendRawMimeMail: %w", err)
	}

	return nil
}

// getCachedToken returns a valid access token, refreshing it if needed.
func (h *GraphMailHandler) getCachedToken() (string, error) {
	h.tokenMutex.Lock()
	defer h.tokenMutex.Unlock()

	now := time.Now().Unix()
	// Refresh if token is missing or expires in <60s
	if h.token == "" || now > h.tokenExp-60 {
		token, err := h.cred.GetToken(h.ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://graph.microsoft.com/.default"},
		})
		if err != nil {
			return "", fmt.Errorf("GetToken: %w", err)
		}
		h.token = token.Token
		h.tokenExp = token.ExpiresOn.Unix()
	}
	return h.token, nil
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
func sendRawMimeMail(ctx context.Context, accessToken string, userID string, mimeMessage []byte) error {
	url := fmt.Sprintf("https://graph.microsoft.com/v1.0/users/%s/sendMail", userID)
	encoded := base64.StdEncoding.EncodeToString(mimeMessage)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(encoded))
	if err != nil {
		return fmt.Errorf("NewRequestWithContext: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http.Do: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sendMail failed: %s\n%s", resp.Status, string(b))
	}
	return nil
}
