package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"strings"
	"testing"
	"time"

	"charm.land/wish/v2/testsession"
	gossh "golang.org/x/crypto/ssh"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/config"
	applog "github.com/mynameis-nigel/ssh-idlefarmer/internal/log"
)

func testServer(t *testing.T, mutate func(*config.Config)) (*Server, string) {
	t.Helper()

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.ListenPort = 0
	cfg.HostKeyPath = t.TempDir() + "/host_key"
	cfg.IdleTimeout = time.Hour
	cfg.MaxSessionsPerKey = 2
	cfg.MaxConnections = 10
	cfg.RateLimitPerSecond = 100
	if mutate != nil {
		mutate(&cfg)
	}

	logger := applog.New("error", "text")
	srv, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
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

func TestPlaceholderShowsSlotAndFingerprint(t *testing.T) {
	_, addr := testServer(t, nil)
	signer := testSigner(t)

	readScreen := func(user string) string {
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

		if err := sess.RequestPty("xterm", 80, 24, gossh.TerminalModes{}); err != nil {
			t.Fatal(err)
		}
		stdout, err := sess.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := sess.Shell(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(300 * time.Millisecond)
		_ = sess.Close()
		b, _ := io.ReadAll(stdout)
		return stripANSI(string(b))
	}

	screenDefault := readScreen("alice")
	if !strings.Contains(screenDefault, "alice") {
		t.Fatalf("expected slot alice in %q", screenDefault)
	}
	if !strings.Contains(screenDefault, "SHA256:") {
		t.Fatalf("expected fingerprint in %q", screenDefault)
	}

	screenOther := readScreen("other")
	if !strings.Contains(screenOther, "other") {
		t.Fatalf("expected slot other in %q", screenOther)
	}
}

func TestDifferentKeysDifferentFingerprints(t *testing.T) {
	_, addr := testServer(t, nil)

	fp := func(signer gossh.Signer) string {
		t.Helper()
		client, err := gossh.Dial("tcp", addr, clientConfig(t, "same", signer))
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		sess, err := client.NewSession()
		if err != nil {
			t.Fatal(err)
		}
		defer sess.Close()
		if err := sess.RequestPty("xterm", 80, 24, gossh.TerminalModes{}); err != nil {
			t.Fatal(err)
		}
		stdout, _ := sess.StdoutPipe()
		_ = sess.Shell()
		time.Sleep(200 * time.Millisecond)
		_ = sess.Close()
		out := stripANSI(readAll(stdout))
		i := strings.Index(out, "SHA256:")
		if i < 0 {
			t.Fatal("no fingerprint")
		}
		end := strings.Index(out[i:], "\n")
		if end < 0 {
			return out[i:]
		}
		return out[i : i+end]
	}

	a := fp(testSigner(t))
	b := fp(testSigner(t))
	if a == b {
		t.Fatal("expected different fingerprints for different keys")
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

func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		if esc {
			if r == 'm' {
				esc = false
			}
			continue
		}
		if r == '\x1b' {
			esc = true
			continue
		}
		if r >= 32 || r == '\n' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
