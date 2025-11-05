package udp

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"

	"math/rand"
	"syscall"
	"time"
)

type ClientConfig struct {
	// Remote is "<host>:<port>", e.g. "192.168.1.234:1234" (Loxone target).
	Remote string

	// WriteTimeout bounds each UDP write. Default 1s.
	WriteTimeout time.Duration

	// QueueSize is the outgoing message buffer. Default 256.
	QueueSize int

	// BaseBackoff and MaxBackoff for reconnect/retry. Defaults: 200ms .. 10s.
	BaseBackoff time.Duration
	MaxBackoff  time.Duration

	// ResolveInterval re-resolves the remote each reconnect. Default: every reconnect.
	ResolveInterval time.Duration

	// Logger (optional). If nil, logs are disabled.
	Logger *slog.Logger
}

type Client struct {
	cfg ClientConfig

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	conn      *net.UDPConn
	remoteUDP *net.UDPAddr

	ch   chan []byte
	wg   sync.WaitGroup
	rand *rand.Rand

	// throttle hostname re-resolution
	lastResolve time.Time
}

func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	cfg = withDefaults(cfg)
	ctx, cancel := context.WithCancel(ctx)

	c := &Client{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		ch:     make(chan []byte, cfg.QueueSize),
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	// initial resolve + dial (non-fatal if it fails; the loop will retry)
	if err := c.resolveAndDial(); err != nil {
		slog.Warn("initial dial failed; will retry in background", "err", err)
	}

	c.wg.Add(1)
	go c.runSender()

	return c, nil
}

func (c *Client) Close() error {
	c.cancel()
	close(c.ch)
	c.wg.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	return nil
}

// Send enqueues a datagram to be sent. It never blocks longer than 1ms.
// If the queue is full, it drops the oldest item (log + keep moving).
func (c *Client) Send(b []byte) {
	if b == nil {
		return
	}
	select {
	case c.ch <- append([]byte(nil), b...):
	default:
		// drop oldest to keep recent signals flowing
		select {
		case <-c.ch:
		default:
		}
		select {
		case c.ch <- append([]byte(nil), b...):
		default:
			// extremely congested; drop new one as well
			slog.Warn("udp queue saturated; dropping message")
		}
	}
}

func (c *Client) runSender() {
	defer c.wg.Done()

	backoff := c.cfg.BaseBackoff

	for {
		select {
		case <-c.ctx.Done():
			return
		case msg, ok := <-c.ch:
			if !ok {
				return
			}

			// ensure we have a connection
			if !c.isConnReady() {
				if err := c.reconnect(backoff); err != nil {
					backoff = c.nextBackoff(backoff)
					slog.Warn("reconnect failed", "err", err, "backoff", backoff.String())
					c.sleep(backoff)
					// requeue attempt: we try send now; if it fails, message may drop after retries below
				} else {
					backoff = c.cfg.BaseBackoff
				}
			}

			// try send with short retry loop
			const maxSendAttempts = 3
			var sent bool
			for attempt := 1; attempt <= maxSendAttempts; attempt++ {
				err := c.write(msg)
				if err == nil {
					sent = true
					backoff = c.cfg.BaseBackoff // reset after success
					break
				}
				if !retryable(err) {
					slog.Warn("udp send non-retryable", "err", err)
					break
				}
				// retry: reconnect + backoff
				slog.Debug("udp send failed; will reconnect and retry",
					"attempt", attempt, "err", err, "backoff", backoff.String())
				_ = c.reconnect(backoff) // error logged inside
				c.sleep(backoff)
				backoff = c.nextBackoff(backoff)
			}
			if !sent {
				slog.Warn("dropping message after retries")
			}
		}
	}
}

func (c *Client) write(b []byte) error {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil {
		return errors.New("no UDP connection")
	}
	_ = conn.SetWriteDeadline(time.Now().Add(c.cfg.WriteTimeout))
	_, err := conn.Write(b)
	return err
}

func (c *Client) reconnect(wait time.Duration) error {
	// Always re-resolve (or at a minimum cadence)
	if c.cfg.ResolveInterval == 0 || time.Since(c.lastResolve) >= c.cfg.ResolveInterval {
		if err := c.resolve(); err != nil {
			slog.Warn("resolve failed", "err", err)
			return err
		}
		c.lastResolve = time.Now()
	}

	// close old connection
	c.mu.Lock()
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
	remote := c.remoteUDP
	c.mu.Unlock()

	// dial
	conn, err := net.DialUDP("udp", nil, remote)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	slog.Info("udp connected", "remote", remote.String())
	return nil
}

func (c *Client) resolveAndDial() error {
	if err := c.resolve(); err != nil {
		return err
	}
	return c.reconnect(0)
}

func (c *Client) resolve() error {
	addr, err := net.ResolveUDPAddr("udp", c.cfg.Remote)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.remoteUDP = addr
	c.mu.Unlock()
	return nil
}

func (c *Client) isConnReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && c.remoteUDP != nil
}

func (c *Client) nextBackoff(curr time.Duration) time.Duration {
	if curr <= 0 {
		curr = c.cfg.BaseBackoff
	}
	next := curr * 2
	if next > c.cfg.MaxBackoff {
		next = c.cfg.MaxBackoff
	}
	// add jitter (+/- 20%)
	j := float64(next) * (0.2 * (c.rand.Float64()*2 - 1)) // [-20%, +20%]
	return time.Duration(float64(next) + j)
}

func (c *Client) sleep(d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-c.ctx.Done():
	case <-timer.C:
	}
}

func retryable(err error) bool {
	var nerr net.Error
	if errors.As(err, &nerr) {
		// timeouts or temporary network failures
		if nerr.Timeout() || nerr.Temporary() {
			return true
		}
	}
	// common syscalls when network/host/port is unreachable
	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE)
}

func withDefaults(cfg ClientConfig) ClientConfig {
	if cfg.WriteTimeout <= 0 {
		cfg.WriteTimeout = time.Second
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 256
	}
	if cfg.BaseBackoff <= 0 {
		cfg.BaseBackoff = 200 * time.Millisecond
	}
	if cfg.MaxBackoff <= 0 {
		cfg.MaxBackoff = 10 * time.Second
	}
	return cfg
}
