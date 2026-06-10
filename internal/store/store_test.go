package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"testing"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func freshPayload(body string) func() ([]byte, int, error) {
	return func() ([]byte, int, error) { return []byte(body), 2, nil }
}

func TestOpenAppliesAllMigrationsAndWAL(t *testing.T) {
	st := openTest(t)
	v, err := st.SchemaVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v != len(migrations) {
		t.Fatalf("schema version = %d, want %d", v, len(migrations))
	}
	var mode string
	if err := st.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}
}

func TestAccountUpsertKeepsFirstSeen(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	if err := st.TouchAccount(ctx, "SHA256:abc", "ssh-ed25519 AAA", 100); err != nil {
		t.Fatal(err)
	}
	if err := st.TouchAccount(ctx, "SHA256:abc", "ssh-ed25519 AAA", 200); err != nil {
		t.Fatal(err)
	}
	var first, last int64
	if err := st.db.QueryRow(
		"SELECT first_seen, last_seen FROM accounts WHERE fingerprint = ?",
		"SHA256:abc").Scan(&first, &last); err != nil {
		t.Fatal(err)
	}
	if first != 100 || last != 200 {
		t.Fatalf("first/last = %d/%d, want 100/200", first, last)
	}
}

func TestSaveLifecycleAndOwnershipIsolation(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	for _, fp := range []string{"SHA256:keyA", "SHA256:keyB"} {
		if err := st.TouchAccount(ctx, fp, "k", 1); err != nil {
			t.Fatal(err)
		}
	}

	// Same slot under two keys: two distinct saves.
	rowA, createdA, err := st.LoadOrCreateSave(ctx, "SHA256:keyA", "farm", 10, freshPayload("stateA"))
	if err != nil {
		t.Fatal(err)
	}
	rowB, createdB, err := st.LoadOrCreateSave(ctx, "SHA256:keyB", "farm", 20, freshPayload("stateB"))
	if err != nil {
		t.Fatal(err)
	}
	if !createdA || !createdB {
		t.Fatal("both saves should be newly created")
	}
	if string(rowA.State) != "stateA" || string(rowB.State) != "stateB" {
		t.Fatal("saves with the same slot crossed keys")
	}

	// Mutate A's save; B's must be untouched, and reload returns the new state.
	if err := st.PersistSave(ctx, "SHA256:keyA", "farm", []byte("stateA2"), 2, 30); err != nil {
		t.Fatal(err)
	}
	rowA2, created, err := st.LoadOrCreateSave(ctx, "SHA256:keyA", "farm", 40, freshPayload("WRONG"))
	if err != nil {
		t.Fatal(err)
	}
	if created || string(rowA2.State) != "stateA2" {
		t.Fatalf("expected persisted stateA2, got created=%v state=%s", created, rowA2.State)
	}
	rowB2, _, err := st.LoadOrCreateSave(ctx, "SHA256:keyB", "farm", 40, freshPayload("WRONG"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rowB2.State) != "stateB" {
		t.Fatal("writing keyA's save changed keyB's save")
	}

	// Multiple slots under one key are isolated from each other.
	if _, _, err := st.LoadOrCreateSave(ctx, "SHA256:keyA", "second", 50, freshPayload("other")); err != nil {
		t.Fatal(err)
	}
	slots, err := st.ListSlots(ctx, "SHA256:keyA")
	if err != nil {
		t.Fatal(err)
	}
	if len(slots) != 2 {
		t.Fatalf("keyA slots = %v, want 2", slots)
	}
	slotsB, _ := st.ListSlots(ctx, "SHA256:keyB")
	if len(slotsB) != 1 {
		t.Fatalf("keyB slots = %v, want 1", slotsB)
	}
}

func TestPersistRequiresExistingRow(t *testing.T) {
	st := openTest(t)
	err := st.PersistSave(context.Background(), "SHA256:ghost", "farm", []byte("x"), 2, 1)
	if err == nil {
		t.Fatal("persisting a nonexistent save must fail, not create rows")
	}
}

func TestHostileSlotAndFingerprintRejected(t *testing.T) {
	st := openTest(t)
	ctx := context.Background()
	bad := []string{"", "UPPER", "a b", "a;drop table saves;--", "../../etc", "x'or'1'='1",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"} // 33 chars
	for _, slot := range bad {
		if _, _, err := st.LoadOrCreateSave(ctx, "SHA256:k", slot, 1, freshPayload("x")); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("slot %q: expected ErrInvalidKey, got %v", slot, err)
		}
		if err := st.PersistSave(ctx, "SHA256:k", slot, []byte("x"), 2, 1); !errors.Is(err, ErrInvalidKey) {
			t.Fatalf("persist slot %q: expected ErrInvalidKey, got %v", slot, err)
		}
	}
	if _, _, err := st.LoadOrCreateSave(ctx, "", "ok", 1, freshPayload("x")); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("empty fingerprint must be rejected")
	}
}

func TestStateSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persist.db")
	ctx := context.Background()

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.TouchAccount(ctx, "SHA256:k", "key", 1); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.LoadOrCreateSave(ctx, "SHA256:k", "farm", 1, freshPayload("before")); err != nil {
		t.Fatal(err)
	}
	if err := st.PersistSave(ctx, "SHA256:k", "farm", []byte("after"), 2, 99); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	row, created, err := st2.LoadOrCreateSave(ctx, "SHA256:k", "farm", 100, freshPayload("WRONG"))
	if err != nil {
		t.Fatal(err)
	}
	if created || string(row.State) != "after" || row.LastActive != 99 {
		t.Fatalf("state lost across restart: created=%v state=%s last=%d", created, row.State, row.LastActive)
	}
}

// TestMigrationUpgradesV1Database simulates a database created before the
// Task 4 migration (schema v1, no state_version column) and confirms Open
// upgrades it without touching existing rows.
func TestMigrationUpgradesV1Database(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.db")
	ctx := context.Background()

	db, err := sql.Open("sqlite", "file:"+url.PathEscape(path)+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO accounts (fingerprint, public_key, first_seen, last_seen) VALUES ('SHA256:old', 'k', 1, 1)"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		"INSERT INTO saves (fingerprint, slot, created_at, last_active, state) VALUES ('SHA256:old', 'farm', 1, 2, 'v1state')"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	st, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("migrating a v1 database failed: %v", err)
	}
	defer st.Close()

	v, _ := st.SchemaVersion(ctx)
	if v != len(migrations) {
		t.Fatalf("schema version = %d, want %d", v, len(migrations))
	}
	row, created, err := st.LoadOrCreateSave(ctx, "SHA256:old", "farm", 100, freshPayload("WRONG"))
	if err != nil {
		t.Fatal(err)
	}
	if created || string(row.State) != "v1state" {
		t.Fatal("migration destroyed existing save data")
	}
	if row.StateVersion != 1 {
		t.Fatalf("pre-migration rows must default to state_version 1, got %d", row.StateVersion)
	}

	// Re-opening is idempotent: no migration reruns, no errors.
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen after migration failed: %v", err)
	}
	_ = st2.Close()
}

func TestNewerSchemaRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.db")
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(path))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", len(migrations)+5)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), path); err == nil {
		t.Fatal("expected refusal to open a newer-schema database")
	}
}
