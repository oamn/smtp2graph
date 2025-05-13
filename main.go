// Package main starts the smtp2graph application, loading configuration and running the SMTP server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/mail"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/emersion/go-smtp"
	"github.com/getsentry/sentry-go"
)

// main loads configuration, initializes Sentry, sets up the SMTP backend, and starts the SMTP server.
func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *versionFlag {
		appName := filepath.Base(os.Args[0])
		fmt.Printf("%s (%s) %s %s/%s\n", appName, revision, runtime.Version(), runtime.GOOS, runtime.GOARCH)

		os.Exit(0)
	}

	cfg, err := LoadConfig()
	if err != nil {
		exitWithError(err)
	}

	// Initialize Sentry error reporting if DSN is configured.
	cleanupSentry := InitSentry(cfg)

	// Create a root context that is canceled on shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Create a new Sentry hub for the context
	// This allows us to use the same hub for all operations in this context
	hub := sentry.CurrentHub().Clone()
	ctx = sentry.SetHubOnContext(ctx, hub)

	defer func() {
		if r := recover(); r != nil {
			sentry.CurrentHub().Recover(r)
			log.Printf("panic: %v", r)
			cleanupSentry(ctx)
			os.Exit(2)
		}
	}()
	defer cancel()
	defer cleanupSentry(ctx)

	// Set up signal handling for graceful shutdown
	shutdownCh := make(chan os.Signal, 1)
	doneCh := make(chan struct{})
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	// Set up the SMTP backend, passing the context to the handler
	handler, err := NewGraphMailHandler(ctx, cfg)
	if err != nil {
		exitWithError(err)
	}

	be := &Backend{
		config:  cfg,
		ctx:     ctx,
		handler: handler,
	}

	// Create and configure the SMTP server instance.
	s := smtp.NewServer(be)
	s.EnableSMTPUTF8 = true
	s.EnableBINARYMIME = true
	s.AllowInsecureAuth = true

	s.Addr = cfg.SMTPAddr
	s.Domain = cfg.SMTPDomain
	s.WriteTimeout = cfg.WriteTimeout
	s.ReadTimeout = cfg.ReadTimeout
	s.MaxMessageBytes = cfg.MaxMessageBytes
	s.MaxRecipients = cfg.MaxRecipients

	go func() {
		<-shutdownCh
		log.Println("Received interrupt signal, shutting down SMTP server...")
		cancel() // cancel context for all in-flight operations
		if err := s.Close(); err != nil {
			log.Printf("Error shutting down SMTP server: %v", err)
		}
		close(doneCh)
	}()

	// Main loop: start the server and wait for shutdown signal
	log.Println("Starting server at", s.Addr)
	if err := s.ListenAndServe(); err != nil && err != smtp.ErrServerClosed {
		exitWithError(err)
	}

	// Wait for shutdown signal to complete cleanup
	<-doneCh
}

// Backend implements the SMTP server methods required by go-smtp.
// Backend holds the handler used for processing messages.
type Backend struct {
	config  *Config
	ctx     context.Context
	handler Handler
}

// NewSession is called after the client greeting (EHLO, HELO) and creates a new SMTP session.
func (bkd *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ctx := bkd.ctx // Use the backend's context directly
	return &Session{
		config:     bkd.config,
		ctx:        ctx,
		handler:    bkd.handler,
		auth:       false,
		sender:     nil,
		recipients: make([]mail.Address, 0, 1),
	}, nil
}

// exitWithError logs, reports, and exits on fatal errors.
func exitWithError(err error) {
	if err == nil {
		return
	}
	ReportError(context.Background(), err)
	log.Printf("fatal: %v", err)
	os.Exit(1)
}
