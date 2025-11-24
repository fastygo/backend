package lifecycle

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// ShutdownFunc describes a graceful shutdown callback.
type ShutdownFunc func(ctx context.Context) error

type hook struct {
	name string
	fn   ShutdownFunc
}

// Manager coordinates graceful shutdown hooks and reacts to OS signals.
type Manager struct {
	timeout time.Duration
	logger  *zap.Logger

	mu    sync.Mutex
	hooks []hook
}

// New creates a lifecycle manager with the desired timeout.
func New(timeout time.Duration, logger *zap.Logger) *Manager {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Manager{
		timeout: timeout,
		logger:  logger,
	}
}

// Register adds a shutdown hook. Hooks are executed in reverse order.
func (m *Manager) Register(name string, fn ShutdownFunc) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.hooks = append(m.hooks, hook{name: name, fn: fn})
}

// Shutdown executes all registered hooks, respecting the configured timeout.
func (m *Manager) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if m.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.timeout)
		defer cancel()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result error
	for i := len(m.hooks) - 1; i >= 0; i-- {
		h := m.hooks[i]
		if err := h.fn(ctx); err != nil {
			m.logger.Error("shutdown hook failed", zap.String("component", h.name), zap.Error(err))
			result = errors.Join(result, err)
			continue
		}
		m.logger.Info("component stopped", zap.String("component", h.name))
	}
	return result
}

// Listen blocks until an OS termination signal is received and then invokes the provided cancel function.
func (m *Manager) Listen(cancel context.CancelFunc) {
	if cancel == nil {
		return
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		defer signal.Stop(sigCh)
		sig := <-sigCh
		m.logger.Info("shutdown signal received", zap.String("signal", sig.String()))
		cancel()
	}()
}
