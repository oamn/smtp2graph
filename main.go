// Package main starts the smtp2graph application, loading configuration and running the SMTP server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/mail"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/getsentry/sentry-go"
)

const shutdownTimeout = 30 * time.Second

// main loads configuration, initializes Sentry, sets up the SMTP backend, and starts the SMTP server.
func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) (exitCode int) {
	flags := flag.NewFlagSet(filepath.Base(os.Args[0]), flag.ContinueOnError)
	versionFlag := flags.Bool("version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *versionFlag {
		appName := filepath.Base(os.Args[0])
		fmt.Printf("%s (%s) %s %s/%s\n", appName, revision, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return 0
	}

	cfg, err := loadConfig()
	if err != nil {
		log.Printf("fatal: %v", err)
		return 1
	}

	cleanupSentry, err := initSentry(cfg)
	if err != nil {
		log.Printf("fatal: initialize Sentry: %v", err)
		return 1
	}
	defer cleanupSentry()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := sentry.CurrentHub().Clone()
	ctx = sentry.SetHubOnContext(ctx, hub)

	defer func() {
		if recovered := recover(); recovered != nil {
			hub.Recover(recovered)
			log.Printf("panic: %v", recovered)
			exitCode = 2
		}
	}()

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer signal.Stop(shutdownCh)

	handler, err := newGraphMailHandler(cfg)
	if err != nil {
		return handleFatalError(ctx, err)
	}

	be := &smtpBackend{
		config:  cfg,
		ctx:     ctx,
		handler: handler,
	}

	s := newSMTPServer(be, cfg)
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return handleFatalError(ctx, err)
	}
	defer listener.Close()

	log.Println("Starting server at", s.Addr)
	if err := serveSMTPServer(s, listener, shutdownCh, cancel, shutdownTimeout); err != nil {
		return handleFatalError(ctx, err)
	}

	return 0
}

func newSMTPServer(be smtp.Backend, cfg *appConfig) *smtp.Server {
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
	return s
}

func serveSMTPServer(
	s *smtp.Server,
	listener net.Listener,
	shutdownCh <-chan os.Signal,
	cancel context.CancelFunc,
	timeout time.Duration,
) error {
	ready := &readyListener{
		Listener: listener,
		ready:    make(chan struct{}),
	}
	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- s.Serve(ready)
	}()

	select {
	case <-ready.ready:
	case err := <-serverErrCh:
		return err
	}

	select {
	case err := <-serverErrCh:
		return err
	case <-shutdownCh:
		log.Println("Received interrupt signal, shutting down SMTP server...")
		shutdownCtx, stopShutdown := context.WithTimeout(context.Background(), timeout)
		shutdownErr := s.Shutdown(shutdownCtx)
		stopShutdown()
		cancel()

		// Shutdown closes listeners before waiting for active sessions, so Serve
		// has exited even when the session drain reaches its deadline.
		serverErr := <-serverErrCh
		if errors.Is(serverErr, smtp.ErrServerClosed) {
			serverErr = nil
		}
		return errors.Join(shutdownErr, serverErr)
	}
}

type readyListener struct {
	net.Listener
	ready chan struct{}
	once  sync.Once
}

func (l *readyListener) Accept() (net.Conn, error) {
	l.once.Do(func() {
		close(l.ready)
	})
	return l.Listener.Accept()
}

// smtpBackend implements the SMTP server methods required by go-smtp.
// smtpBackend holds the handler used for processing messages.
type smtpBackend struct {
	config  *appConfig
	ctx     context.Context
	handler messageHandler
}

// NewSession is called after the client greeting (EHLO, HELO) and creates a new SMTP session.
func (bkd *smtpBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	ctx := bkd.ctx // Use the backend's context directly
	return &smtpSession{
		config:     bkd.config,
		ctx:        ctx,
		handler:    bkd.handler,
		auth:       false,
		sender:     nil,
		recipients: make([]mail.Address, 0, 1),
	}, nil
}

func handleFatalError(ctx context.Context, err error) int {
	reportError(ctx, err)
	log.Printf("fatal: %v", err)
	return 1
}
