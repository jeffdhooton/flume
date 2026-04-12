// Package daemon is the long-running flume process. It owns the Unix socket,
// the reverse proxy, the BadgerDB store, and the RPC dispatcher.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/jeffdhooton/flume/internal/rpc"
	"github.com/jeffdhooton/flume/internal/store"
)

// logWriter is stderr — used by the proxy for non-fatal errors.
var logWriter = os.Stderr

// DefaultShutdownGrace is the time from SIGTERM to forceful close.
const DefaultShutdownGrace = 5 * time.Second

// Config holds the daemon's runtime configuration.
type Config struct {
	ProxyPort  int    // port to listen on for HTTP proxy (default 8089)
	TargetAddr string // upstream dev server address (default localhost:8000)
	Retention  time.Duration
	MaxRequests int
}

// DefaultConfig returns sane defaults.
func DefaultConfig() Config {
	return Config{
		ProxyPort:   8089,
		TargetAddr:  "localhost:8000",
		Retention:   30 * time.Minute,
		MaxRequests: 1000,
	}
}

// Daemon is one running flumed process.
type Daemon struct {
	layout  Layout
	config  Config
	server  *rpc.Server
	store   *store.Store
	startAt time.Time

	mu       sync.Mutex
	listener net.Listener
}

// New constructs a Daemon. Call Run to begin serving.
func New(layout Layout, cfg Config) *Daemon {
	d := &Daemon{
		layout: layout,
		config: cfg,
		server: rpc.NewServer(),
	}
	d.registerMethods()
	return d
}

// Run takes ownership of the process: writes PID, opens Unix socket, serves
// RPC until ctx is cancelled or SIGTERM/SIGINT.
func (d *Daemon) Run(ctx context.Context) error {
	d.startAt = time.Now()

	if err := os.MkdirAll(d.layout.Home, 0o755); err != nil {
		return fmt.Errorf("ensure home: %w", err)
	}

	if alive, pid := AliveDaemon(d.layout); alive {
		return fmt.Errorf("flume daemon already running (pid %d, socket %s)", pid, d.layout.SocketPath)
	}

	// Remove stale socket from a previous crash.
	if err := os.Remove(d.layout.SocketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", d.layout.SocketPath)
	if err != nil {
		return fmt.Errorf("listen unix %s: %w", d.layout.SocketPath, err)
	}
	if err := os.Chmod(d.layout.SocketPath, 0o600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("chmod socket: %w", err)
	}
	d.mu.Lock()
	d.listener = ln
	d.mu.Unlock()

	if err := os.WriteFile(d.layout.PIDPath, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		_ = ln.Close()
		return fmt.Errorf("write pid file: %w", err)
	}
	defer os.Remove(d.layout.PIDPath)
	defer os.Remove(d.layout.SocketPath)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sigCh)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-runCtx.Done():
		}
	}()

	// Open the request store.
	st, err := store.Open(store.Options{
		Dir:        d.layout.DataDir,
		TTL:        d.config.Retention,
		MaxEntries: d.config.MaxRequests,
	})
	if err != nil {
		_ = ln.Close()
		return fmt.Errorf("open store: %w", err)
	}
	d.store = st
	defer d.store.Close()

	// Start the reverse proxy HTTP server.
	proxyServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", d.config.ProxyPort),
		Handler: d.newProxy(),
	}
	go func() {
		if err := proxyServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "flumed: proxy server error: %v\n", err)
		}
	}()
	defer func() {
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = proxyServer.Shutdown(shutCtx)
	}()

	fmt.Fprintf(os.Stderr, "flumed: listening on %s (proxy :%d -> %s)\n",
		d.layout.SocketPath, d.config.ProxyPort, d.config.TargetAddr)

	serveErr := d.server.Serve(runCtx, ln)
	if serveErr != nil && !errors.Is(serveErr, net.ErrClosed) {
		return serveErr
	}
	return nil
}
