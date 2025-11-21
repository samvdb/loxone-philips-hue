package udp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	conn   *net.UDPConn
	log    *slog.Logger
	handle CommandHandler

	readBuf int
}

// CommandHandler receives parsed commands and should call Hue.
type CommandHandler interface {
	Apply(ctx context.Context, cmd Command) error
}

type Command struct {
	Domain string // "light"
	ID     string // hue resource id (UUID-ish for v2)
	Action string // "on" | "dimmable"
	Value  string // raw value e.g. "true", "75"
}

type ServerConfig struct {
	ListenAddr *net.UDPAddr
	Handler    CommandHandler
	Logger     *slog.Logger
	ReadBuf    int // bytes, default 2k
}

func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.ListenAddr == nil {
		return nil, errors.New("ListenAddr required")
	}
	if cfg.Handler == nil {
		return nil, errors.New("Handler required")
	}
	if cfg.ReadBuf <= 0 {
		cfg.ReadBuf = 2048
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	conn, err := net.ListenUDP("udp", cfg.ListenAddr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}
	return &Server{
		conn:    conn,
		log:     cfg.Logger.With("module", "udpserver", "addr", cfg.ListenAddr.String()),
		handle:  cfg.Handler,
		readBuf: cfg.ReadBuf,
	}, nil
}

func (s *Server) Close() error {
	return s.conn.Close()
}

// Run loops until ctx is cancelled. It sets short deadlines to make cancellation responsive.
func (s *Server) Run(ctx context.Context) error {
	defer s.conn.Close()
	s.log.Info("udp server started")
	buf := make([]byte, s.readBuf)
	for {
		// Make ReadFromUDP interruptible via deadline.
		_ = s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				// check ctx and continue
				select {
				case <-ctx.Done():
					s.log.Info("udp server stopping (context cancelled)")
					return ctx.Err()
				default:
					continue
				}
			}

			// If ctx is cancelled, treat any read error as shutdown.
			select {
			case <-ctx.Done():
				s.log.Info("udp server stopping (context cancelled)")
				return ctx.Err()
			default:
			}
			return fmt.Errorf("read udp: %w", err)
		}

		line := string(bytes.TrimSpace(buf[:n]))
		if line == "" {
			continue
		}

		cmd, perr := parseCommand(line)
		if perr != nil {
			s.log.Warn("invalid command", "from", addr.String(), "line", line, "error", perr.Error())
			continue
		}

		// Handle in-line; UDP is cheap—if needed later, you can add a worker pool.
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		slog.Info("applying command", "domain", cmd.Domain, "action", cmd.Action, "id", cmd.ID, "value", cmd.Value)
		err = s.handle.Apply(callCtx, cmd)
		cancel()
		if err != nil {
			s.log.Error("apply failed", "cmd", fmt.Sprintf("%+v", cmd), "error", err.Error())
			continue
		}
		s.log.Debug("command applied", "from", addr.String(), "cmd", fmt.Sprintf("%+v", cmd))
	}
}

// /grouped_light/<id>/on true
// /grouped_light/<id>/dimmable 75
func parseCommand(line string) (Command, error) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return Command{}, fmt.Errorf("expected '<path> <value>'")
	}
	path, value := parts[0], parts[1]

	segs := strings.Split(strings.Trim(path, " \t\r\n"), "/")
	// ["", "light", "<id>", "on"]  or  ["", "light", "<id>", "dimmable"]
	if len(segs) < 4 || segs[0] != "" {
		return Command{}, fmt.Errorf("bad path: %s", path)
	}

	cmd := Command{
		Domain: segs[1],
		ID:     segs[2],
		Action: segs[3],
		Value:  value,
	}

	// basic validation
	switch cmd.Domain {
	case "grouped_light":
	default:
		return Command{}, fmt.Errorf("unsupported domain: %s", cmd.Domain)
	}
	switch cmd.Action {
	case "on":
		v := strings.ToLower(cmd.Value)
		if v != "true" && v != "false" && v != "1" && v != "0" {
			return Command{}, fmt.Errorf("on expects true|false|1|0")
		}
	case "dimmable":
		n, err := strconv.Atoi(cmd.Value)
		if err != nil || n < 0 || n > 100 {
			return Command{}, fmt.Errorf("dimmable expects 0..100")
		}
	default:
		return Command{}, fmt.Errorf("unsupported action: %s", cmd.Action)
	}

	return cmd, nil
}
