package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/wish/v2/testsession"
	gossh "golang.org/x/crypto/ssh"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/config"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/content"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/game"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/identity"
	applog "github.com/mynameis-nigel/ssh-idlefarmer/internal/log"
	"github.com/mynameis-nigel/ssh-idlefarmer/internal/store"
)

func testServer(t *testing.T, mutate func(*config.Config)) (*Server, string) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cfg.ListenPort = 0
	cfg.HostKeyPath = filepath.Join(dir, "host_key")
	cfg.DBPath = filepath.Join(dir, "game.db")
	cfg.IdleTimeout = time.Hour
	cfg.MaxSessionsPerKey = 2
	cfg.MaxConnections = 10
	cfg.RateLimitPerSecond = 100
	if mutate != nil {
		mutate(&cfg)
	}

	logger := applog.New("error", "text")

	st, err := store.Open(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	c, err := content.Load("")
	if err != nil {
		t.Fatal(err)
	}
	games := game.NewManager(st, c, logger, cfg.AutosaveInterval, game.Policy(cfg.SessionPolicy))

	srv, err := New(cfg, logger, games)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = games.Shutdown(ctx)
		_ = srv.Shutdown(ctx)
	})

	addr := testsession.Listen(t, srv.ssh)
	return srv, addr
}

func testSigner(t *testing.T) gossh.Signer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func clientConfig(t *testing.T, user string, signer gossh.Signer) *gossh.ClientConfig {
	t.Helper()
	return &gossh.ClientConfig{
		User:            user,
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec // test only
		Timeout:         5 * time.Second,
	}
}

func TestRejectsPasswordAuth(t *testing.T) {
	_, addr := testServer(t, nil)

	_, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User:            "alice",
		Auth:            []gossh.AuthMethod{gossh.Password("secret")},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         5 * time.Second,
	})
	if err == nil {
		t.Fatal("expected password auth to fail")
	}
}

func TestRejectsSessionWithoutPTY(t *testing.T) {
	_, addr := testServer(t, nil)
	signer := testSigner(t)

	sess, err := testsession.NewClientSession(t, addr, clientConfig(t, "alice", signer))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	sess.Stdout = &out
	if err := sess.Run(""); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "interactive terminal") {
		t.Fatalf("expected PTY message, got: %q", out.String())
	}
}

func readScreen(t *testing.T, addr, user string, signer gossh.Signer) string {
	t.Helper()
	client, err := gossh.Dial("tcp", addr, clientConfig(t, user, signer))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	// gossh.RequestPty takes height before width.
	if err := sess.RequestPty("xterm", 30, 100, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond)
	_ = sess.Close()
	b, _ := io.ReadAll(stdout)
	return stripANSI(string(b))
}

func TestGameShowsTitleAndSlot(t *testing.T) {
	_, addr := testServer(t, nil)
	signer := testSigner(t)

	screenDefault := readScreen(t, addr, "alice", signer)
	if !strings.Contains(screenDefault, "ssh-idlefarmer") {
		t.Fatalf("expected game title in %q", screenDefault)
	}
	if !strings.Contains(screenDefault, "alice") {
		t.Fatalf("expected slot alice in %q", screenDefault)
	}

	screenOther := readScreen(t, addr, "other", signer)
	if !strings.Contains(screenOther, "other") {
		t.Fatalf("expected slot other in %q", screenOther)
	}
}

// TestDifferentKeysGetDistinctSaves connects two different keys with the
// same username and confirms two isolated save rows exist, one per key.
func TestDifferentKeysGetDistinctSaves(t *testing.T) {
	var dbPath string
	_, addr := testServer(t, func(c *config.Config) {
		dbPath = c.DBPath
	})

	signerA, signerB := testSigner(t), testSigner(t)
	_ = readScreen(t, addr, "same", signerA)
	_ = readScreen(t, addr, "same", signerB)

	st, err := store.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	for _, signer := range []gossh.Signer{signerA, signerB} {
		fp := identity.Fingerprint(signer.PublicKey())
		slots, err := st.ListSlots(context.Background(), fp)
		if err != nil {
			t.Fatal(err)
		}
		if len(slots) != 1 || slots[0] != "same" {
			t.Fatalf("key %s slots = %v, want [same]", fp, slots)
		}
	}
}

func TestGlobalSessionCap(t *testing.T) {
	_, addr := testServer(t, func(c *config.Config) {
		c.MaxConnections = 1
		c.MaxSessionsPerKey = 10
	})

	hold := func(signer gossh.Signer) {
		t.Helper()
		client, err := gossh.Dial("tcp", addr, clientConfig(t, "u1", signer))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { client.Close() })
		sess, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		if err := sess.RequestPty("xterm", 80, 24, gossh.TerminalModes{}); err != nil {
			t.Fatal(err)
		}
		if err := sess.Shell(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	hold(testSigner(t))

	client2, err := gossh.Dial("tcp", addr, clientConfig(t, "u2", testSigner(t)))
	if err != nil {
		t.Fatal(err)
	}
	defer client2.Close()
	sess2, err := client2.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sess2.Stdout = &buf
	_ = sess2.Run("")
	if !strings.Contains(buf.String(), "Too many active sessions") {
		t.Fatalf("expected global cap message, got %q", buf.String())
	}
}

func TestRateLimitRejectsBurst(t *testing.T) {
	_, addr := testServer(t, func(c *config.Config) {
		c.RateLimitPerSecond = 0.01
		c.RateLimitBurst = 1
		c.MaxConnections = 50
	})
	signer := testSigner(t)
	cfg := clientConfig(t, "rl", signer)

	denied := false
	for i := 0; i < 8; i++ {
		client, err := gossh.Dial("tcp", addr, cfg)
		if err != nil {
			t.Fatal(err)
		}
		sess, err := client.NewSession()
		if err != nil {
			client.Close()
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		sess.Stderr = &stderr
		_ = sess.Run("")
		client.Close()
		if strings.Contains(stderr.String(), "rate limit") {
			denied = true
			break
		}
	}
	if !denied {
		t.Fatal("expected at least one session to hit the rate limiter")
	}
}

func TestPerKeySessionCap(t *testing.T) {
	_, addr := testServer(t, func(c *config.Config) {
		c.MaxSessionsPerKey = 1
	})
	signer := testSigner(t)
	cfg := clientConfig(t, "captest", signer)

	client1, err := gossh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client1.Close()

	sess1, err := client1.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := sess1.RequestPty("xterm", 80, 24, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	if err := sess1.Shell(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	client2, err := gossh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client2.Close()

	sess2, err := client2.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	sess2.Stdout = &buf
	_ = sess2.Run("") // exits 1 when capped
	if !strings.Contains(buf.String(), "Too many active sessions") {
		t.Fatalf("expected cap message, got %q", buf.String())
	}
}

func readAll(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}

// stripANSI removes escape sequences: CSI (ESC [ ... final byte in @-~),
// OSC (ESC ] ... BEL or ESC \), and two-byte ESC sequences.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		c := s[i]
		if c != '\x1b' {
			if c >= 32 || c == '\n' {
				b.WriteByte(c)
			}
			i++
			continue
		}
		i++ // consume ESC
		if i >= len(s) {
			break
		}
		switch s[i] {
		case '[': // CSI: parameters then a final byte in @-~
			i++
			for i < len(s) && (s[i] < '@' || s[i] > '~') {
				i++
			}
			i++ // final byte
		case ']': // OSC: until BEL or ESC \
			i++
			for i < len(s) && s[i] != '\a' {
				if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '\\' {
					i++
					break
				}
				i++
			}
			i++
		default: // two-byte escape
			i++
		}
	}
	return b.String()
}
