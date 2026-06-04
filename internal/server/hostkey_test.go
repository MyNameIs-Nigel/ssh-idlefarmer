package server

import (
	"os"
	"testing"

	"github.com/mynameis-nigel/ssh-idlefarmer/internal/config"
	applog "github.com/mynameis-nigel/ssh-idlefarmer/internal/log"
)

func TestHostKeyPersistedAcrossRestarts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := dir + "/host_key"
	logger := applog.New("error", "text")

	cfg := config.Config{
		ListenHost:          "127.0.0.1",
		ListenPort:          0,
		HostKeyPath:         path,
		IdleTimeout:         0,
		MaxSessionsPerKey:   2,
		MaxConnections:      10,
		DefaultSlot:         "default",
		RateLimitPerSecond:  100,
		RateLimitBurst:      10,
		RateLimitMaxEntries: 100,
	}

	srv1, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = srv1

	srv2, err := New(cfg, logger)
	if err != nil {
		t.Fatal(err)
	}
	_ = srv2

	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("host key changed across server restarts")
	}
}
