package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"charm.land/wish/v2"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"charm.land/wish/v2/ratelimiter"
	"github.com/charmbracelet/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/time/rate"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/config"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/game"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/tui"
)

// sessionKey carries per-connection game state through the ssh.Context.
type sessionKey struct{}

type sessionState struct {
	id  identity.SessionIdentity
	res game.AttachResult
}

// Server wraps the Wish SSH server and related middleware.
type Server struct {
	cfg    config.Config
	logger *slog.Logger
	ssh    *ssh.Server
	games  *game.Manager
}

// New constructs and configures the SSH server over the given save manager.
func New(cfg config.Config, logger *slog.Logger, games *game.Manager) (*Server, error) {
	if err := ensureHostKeyDir(cfg.HostKeyPath); err != nil {
		return nil, err
	}

	limits := NewSessionLimits(cfg.MaxConnections, cfg.MaxSessionsPerKey)
	rl := ratelimiter.NewRateLimiter(
		rate.Limit(cfg.RateLimitPerSecond),
		cfg.RateLimitBurst,
		cfg.RateLimitMaxEntries,
	)

	srv := &Server{cfg: cfg, logger: logger, games: games}

	s, err := wish.NewServer(
		wish.WithAddress(cfg.ListenAddr()),
		wish.WithHostKeyPath(cfg.HostKeyPath),
		wish.WithIdleTimeout(cfg.IdleTimeout),
		wish.WithPublicKeyAuth(func(_ ssh.Context, key ssh.PublicKey) bool {
			return key != nil
		}),
		// Middlewares run bottom-up: logging → rate limit → caps → PTY
		// requirement → save attach → the game UI. The Bubble Tea handler
		// is the only thing a session can ever reach — there is no shell.
		wish.WithMiddleware(
			bubbletea.Middleware(srv.teaHandler),
			srv.attachSave(),
			RequirePTY(),
			limits.Middleware(),
			ratelimiter.Middleware(rl),
			logging.Middleware(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create ssh server: %w", err)
	}

	srv.ssh = s
	return srv, nil
}

// attachSave resolves the session's identity, opens its save through the
// manager (applying the concurrency policy), and cleans up on disconnect.
func (srv *Server) attachSave() func(ssh.Handler) ssh.Handler {
	return func(next ssh.Handler) ssh.Handler {
		return func(s ssh.Session) {
			id := identity.FromSession(s, srv.cfg.DefaultSlot)
			pub := ""
			if key := s.PublicKey(); key != nil {
				pub = string(gossh.MarshalAuthorizedKey(key))
			}

			res, err := srv.games.Attach(s.Context(), id, pub, time.Now().Unix(), nil)
			if err != nil {
				srv.logger.Warn("attach failed",
					"fingerprint", id.Fingerprint, "slot", id.Slot, "error", err)
				if errors.Is(err, game.ErrSaveBusy) {
					_, _ = io.WriteString(s,
						"🌾 This farm is already open in another session.\r\n"+
							"Close it there (or wait a moment) and reconnect.\r\n")
				} else {
					_, _ = io.WriteString(s,
						"🌧 The farm could not be opened just now. Please try again shortly.\r\n")
				}
				s.Exit(1)
				return
			}
			defer res.Session.Detach()

			srv.logger.Info("session start",
				"fingerprint", id.Fingerprint,
				"slot", id.Slot,
				"remote", s.RemoteAddr().String(),
			)
			s.Context().SetValue(sessionKey{}, &sessionState{id: id, res: res})
			next(s)
		}
	}
}

// teaHandler builds the per-session Bubble Tea program.
func (srv *Server) teaHandler(s ssh.Session) (tui.Model, []tui.ProgramOption) {
	state, _ := s.Context().Value(sessionKey{}).(*sessionState)
	if state == nil {
		// attachSave always runs first; this is unreachable in practice.
		return nil, nil
	}
	width, height := 80, 24
	if pty, _, ok := s.Pty(); ok {
		width, height = pty.Window.Width, pty.Window.Height
	}
	// Idle disconnect is enforced inside the UI: the once-a-second render
	// keeps the transport busy, so a connection-level idle timer would
	// never fire. Only key presses count as activity.
	idleSecs := int64(srv.cfg.IdleTimeout / time.Second)
	return tui.NewGame(state.id, state.res, srv.games.Content(), width, height, time.Now().Unix(), idleSecs), nil
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
