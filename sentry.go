// Package main provides Sentry error reporting integration for smtp2graph.
package main

import (
	"context"
	"time"

	"github.com/getsentry/sentry-go"
)

// initSentry initializes Sentry if a DSN is configured.
// Returns a cleanup function to flush events, or a no-op if Sentry is not enabled.
func initSentry(cfg *appConfig) (func(), error) {
	if cfg.SentryDSN == "" {
		return func() {}, nil
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:     cfg.SentryDSN,
		Release: "smtp2graph@" + revision,
	})
	if err != nil {
		return nil, err
	}
	return func() {
		sentry.Flush(2 * time.Second)
	}, nil
}

// reportError sends an error to Sentry if initialized.
func reportError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.CaptureException(err)
}
