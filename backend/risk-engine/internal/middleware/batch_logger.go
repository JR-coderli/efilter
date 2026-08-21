package middleware

import (
	"context"
	"sync"
	"time"

	"risk-engine/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AccessLogBatcher buffers access logs and flushes them to PostgreSQL in batches.
type AccessLogBatcher struct {
	db       *gorm.DB
	log      *zap.Logger
	buffer   []models.AccessLog
	mu       sync.Mutex
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
}

// NewAccessLogBatcher creates a new batcher. interval controls how often logs are flushed.
func NewAccessLogBatcher(db *gorm.DB, log *zap.Logger, interval time.Duration) *AccessLogBatcher {
	b := &AccessLogBatcher{
		db:       db,
		log:      log,
		buffer:   make([]models.AccessLog, 0, 1024),
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	go b.loop()
	return b
}

// Add appends a log record to the buffer. If the batcher has no DB, the
// record is silently dropped so that core APIs remain available.
func (b *AccessLogBatcher) Add(record models.AccessLog) {
	if b.db == nil {
		return
	}
	b.mu.Lock()
	b.buffer = append(b.buffer, record)
	b.mu.Unlock()
}

// Stop flushes remaining logs and stops the background loop. Safe to call
// even when the batcher has no DB.
func (b *AccessLogBatcher) Stop() {
	close(b.stop)
	<-b.done
}

func (b *AccessLogBatcher) loop() {
	if b.db == nil {
		// Nothing to flush; wait for Stop signal to avoid busy loop.
		<-b.stop
		close(b.done)
		return
	}

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	defer close(b.done)

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.stop:
			b.flush()
			return
		}
	}
}

func (b *AccessLogBatcher) flush() {
	if b.db == nil {
		return
	}

	b.mu.Lock()
	if len(b.buffer) == 0 {
		b.mu.Unlock()
		return
	}
	batch := make([]models.AccessLog, len(b.buffer))
	copy(batch, b.buffer)
	b.buffer = b.buffer[:0]
	b.mu.Unlock()

	if err := b.db.WithContext(context.Background()).CreateInBatches(batch, 500).Error; err != nil {
		b.log.Error("failed to flush access logs", zap.Error(err), zap.Int("count", len(batch)))
	} else {
		b.log.Info("access logs flushed", zap.Int("count", len(batch)))
	}
}
