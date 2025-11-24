package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	redislib "github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/e-go/backend/internal/infrastructure/buffer"
)

type Monitor struct {
	pg     *pgxpool.Pool
	redis  *redislib.Client
	buffer *buffer.Store

	status   Status
	mu       sync.RWMutex
	interval time.Duration
	stopCh   chan struct{}
	logger   *zap.Logger
}

func New(pg *pgxpool.Pool, redis *redislib.Client, buf *buffer.Store, interval time.Duration, logger *zap.Logger) *Monitor {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Monitor{
		pg:       pg,
		redis:    redis,
		buffer:   buf,
		interval: interval,
		stopCh:   make(chan struct{}),
		logger:   logger,
	}
}

func (m *Monitor) Start() {
	go m.loop()
}

func (m *Monitor) Stop() {
	close(m.stopCh)
}

func (m *Monitor) IsOnline() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status.PostgreSQL && m.status.Redis
}

func (m *Monitor) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Monitor) loop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	m.refresh()
	for {
		select {
		case <-ticker.C:
			m.refresh()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Monitor) refresh() {
	bufferOK, bufferSize := m.checkBuffer()
	status := Status{
		PostgreSQL: m.checkPostgres(),
		Redis:      m.checkRedis(),
		Buffer:     bufferOK,
		BufferSize: bufferSize,
		LastCheck:  time.Now(),
	}

	m.mu.Lock()
	m.status = status
	m.mu.Unlock()
}

func (m *Monitor) checkPostgres() bool {
	if m.pg == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return m.pg.Ping(ctx) == nil
}

func (m *Monitor) checkRedis() bool {
	if m.redis == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return m.redis.Ping(ctx).Err() == nil
}

func (m *Monitor) checkBuffer() (bool, int) {
	if m.buffer == nil {
		return false, 0
	}
	size, err := m.buffer.Size()
	if err != nil {
		m.logger.Warn("buffer size check failed", zap.Error(err))
		return false, size
	}
	return true, size
}
