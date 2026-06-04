package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/charmbracelet/ssh"
	"charm.land/wish/v2"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"charm.land/wish/v2/ratelimiter"
	"golang.org/x/time/rate"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/config"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/tui"
)

// Server wraps the Wish SSH server and related middleware.
type Server struct {
	cfg    config.Config
	logger *slog.Logger
	ssh    *ssh.Server
}

// New constructs and configures the SSH server.
func New(cfg config.Config, logger *slog.Logger) (*Server, error) {
	if err := ensureHostKeyDir(cfg.HostKeyPath); err != nil {
		return nil, err
	}

	limits := NewSessionLimits(cfg.MaxConnections, cfg.MaxSessionsPerKey)
	rl := ratelimiter.NewRateLimiter(
		rate.Limit(cfg.RateLimitPerSecond),
		cfg.RateLimitBurst,
		cfg.RateLimitMaxEntries,
	)

	teaHandler := func(s ssh.Session) (tui.Model, []tui.ProgramOption) {
		id := identity.FromSession(s, cfg.DefaultSlot)
		logger.Info("session start",
			"fingerprint", id.Fingerprint,
			"slot", id.Slot,
			"remote", s.RemoteAddr().String(),
		)
		pty, _, _ := s.Pty()
		return tui.NewPlaceholder(id, pty.Window.Width, pty.Window.Height), nil
	}

	s, err := wish.NewServer(
		wish.WithAddress(cfg.ListenAddr()),
		wish.WithHostKeyPath(cfg.HostKeyPath),
		wish.WithIdleTimeout(cfg.IdleTimeout),
		wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool {
			return key != nil
		}),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			RequirePTY(),
			limits.Middleware(),
			ratelimiter.Middleware(rl),
			logging.Middleware(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create ssh server: %w", err)
	}

	return &Server{cfg: cfg, logger: logger, ssh: s}, nil
}

// ListenAndServe starts accepting SSH connections.
func (s *Server) ListenAndServe() error {
	s.logger.Info("ssh server listening", "addr", s.cfg.ListenAddr())
	return s.ssh.ListenAndServe()
}

// Shutdown stops accepting new connections and waits for existing ones.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("ssh server shutting down")
	return s.ssh.Shutdown(ctx)
}

func ensureHostKeyDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o700)
}
