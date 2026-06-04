package config

import (
	"strings"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("IDLEFARM_LISTEN_PORT", "")
	t.Setenv("IDLEFARM_DEFAULT_SLOT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ListenPort != 22 {
		t.Fatalf("ListenPort = %d, want 22", cfg.ListenPort)
	}
	if cfg.DefaultSlot != "default" {
		t.Fatalf("DefaultSlot = %q, want default", cfg.DefaultSlot)
	}
}

func TestLoadRejectsInvalidDefaultSlot(t *testing.T) {
	t.Setenv("IDLEFARM_DEFAULT_SLOT", "!!!")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid default slot")
	}
}

func TestLoadRejectsInvalidEnvInt(t *testing.T) {
	t.Setenv("IDLEFARM_LISTEN_PORT", "not-a-port")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "IDLEFARM_LISTEN_PORT") {
		t.Fatalf("expected listen port error, got %v", err)
	}
}
