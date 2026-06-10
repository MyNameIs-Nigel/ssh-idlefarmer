// Package store is the SQLite persistence layer. It owns all SQL in the
// project: a versioned schema with forward migrations, accounts keyed by
// public-key fingerprint, and saves keyed by (fingerprint, slot).
//
// Ownership is enforced structurally: every query that touches a save takes
// the fingerprint as a parameter, there is no API that addresses a save by
// slot alone, and all access uses parameterized statements.
//
// Write serialization: the pool is capped at a single connection, so the
// database never sees concurrent writes from this process. Per-save mutation
// is additionally serialized by the save actor (internal/game), which is the
// sole writer of its save's row while active.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	_ "modernc.org/sqlite" // pure-Go, cgo-free driver (static builds)
)

// slotPattern re-validates slots at the storage boundary even though Task 1
// sanitizes them at the door — defense in depth.
var slotPattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// ErrInvalidKey is returned when a fingerprint or slot fails validation.
var ErrInvalidKey = errors.New("store: invalid fingerprint or slot")

// Store is the open database handle.
type Store struct {
	db *sql.DB
}

// SaveRow is one persisted save.
type SaveRow struct {
	Fingerprint  string
	Slot         string
	CreatedAt    int64
	LastActive   int64
	State        []byte
	StateVersion int
}

// Open opens (creating if needed) the database at path, applies pending
// migrations, and returns the store. The file and its directory are created
// with restrictive permissions: the database holds all player state.
func Open(ctx context.Context, path string) (*Store, error) {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("store: create db dir: %w", err)
		}
	}

	dsn := "file:" + url.PathEscape(path) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// A single connection serializes all reads and writes (see package doc).
	db.SetMaxOpenConns(1)

	st := &Store{db: db}
	if err := st.verifyWAL(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := st.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Best-effort permission tightening (chmod is advisory on Windows).
	_ = os.Chmod(path, 0o600)
	return st, nil
}

// Close closes the underlying database.
func (st *Store) Close() error { return st.db.Close() }

func (st *Store) verifyWAL(ctx context.Context) error {
	var mode string
	if err := st.db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("store: read journal mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("store: WAL mode required, database reports %q", mode)
	}
	return nil
}

// TouchAccount records that fingerprint connected at now, creating the
// account row on first sight (trust-on-first-use). publicKey is stored in
// authorized_keys format for auditing.
func (st *Store) TouchAccount(ctx context.Context, fingerprint, publicKey string, now int64) error {
	if fingerprint == "" {
		return ErrInvalidKey
	}
	_, err := st.db.ExecContext(ctx, `
		INSERT INTO accounts (fingerprint, public_key, first_seen, last_seen)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (fingerprint) DO UPDATE SET last_seen = excluded.last_seen`,
		fingerprint, publicKey, now, now)
	if err != nil {
		return fmt.Errorf("store: touch account: %w", err)
	}
	return nil
}

// LoadOrCreateSave returns the save for (fingerprint, slot), creating it
// with the payload from fresh() the first time this key uses this slot.
// It can only ever return a save owned by fingerprint.
func (st *Store) LoadOrCreateSave(ctx context.Context, fingerprint, slot string, now int64, fresh func() ([]byte, int, error)) (SaveRow, bool, error) {
	if err := validateKeys(fingerprint, slot); err != nil {
		return SaveRow{}, false, err
	}

	row, err := st.loadSave(ctx, fingerprint, slot)
	if err == nil {
		return row, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return SaveRow{}, false, err
	}

	state, version, err := fresh()
	if err != nil {
		return SaveRow{}, false, fmt.Errorf("store: build fresh save: %w", err)
	}
	_, err = st.db.ExecContext(ctx, `
		INSERT INTO saves (fingerprint, slot, created_at, last_active, state, state_version)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (fingerprint, slot) DO NOTHING`,
		fingerprint, slot, now, now, state, version)
	if err != nil {
		return SaveRow{}, false, fmt.Errorf("store: create save: %w", err)
	}
	// Re-read so a concurrent creator's row (rather than ours) wins cleanly.
	row, err = st.loadSave(ctx, fingerprint, slot)
	if err != nil {
		return SaveRow{}, false, err
	}
	created := row.CreatedAt == now && row.LastActive == now
	return row, created, nil
}

func (st *Store) loadSave(ctx context.Context, fingerprint, slot string) (SaveRow, error) {
	row := SaveRow{Fingerprint: fingerprint, Slot: slot}
	err := st.db.QueryRowContext(ctx, `
		SELECT created_at, last_active, state, state_version
		FROM saves WHERE fingerprint = ? AND slot = ?`,
		fingerprint, slot).
		Scan(&row.CreatedAt, &row.LastActive, &row.State, &row.StateVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SaveRow{}, err
		}
		return SaveRow{}, fmt.Errorf("store: load save: %w", err)
	}
	return row, nil
}

// PersistSave writes the save payload for (fingerprint, slot). It never
// creates rows: persisting a save that was deleted out from under us is an
// error, not a resurrection.
func (st *Store) PersistSave(ctx context.Context, fingerprint, slot string, state []byte, stateVersion int, lastActive int64) error {
	if err := validateKeys(fingerprint, slot); err != nil {
		return err
	}
	res, err := st.db.ExecContext(ctx, `
		UPDATE saves SET state = ?, state_version = ?, last_active = ?
		WHERE fingerprint = ? AND slot = ?`,
		state, stateVersion, lastActive, fingerprint, slot)
	if err != nil {
		return fmt.Errorf("store: persist save: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: persist save: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: persist save: no row for this key/slot")
	}
	return nil
}

// ListSlots returns the slot names owned by fingerprint, newest activity
// first. Used for diagnostics and the stats screen; never crosses keys.
func (st *Store) ListSlots(ctx context.Context, fingerprint string) ([]string, error) {
	if fingerprint == "" {
		return nil, ErrInvalidKey
	}
	rows, err := st.db.QueryContext(ctx, `
		SELECT slot FROM saves WHERE fingerprint = ? ORDER BY last_active DESC`,
		fingerprint)
	if err != nil {
		return nil, fmt.Errorf("store: list slots: %w", err)
	}
	defer rows.Close()
	var slots []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, fmt.Errorf("store: list slots: %w", err)
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

func validateKeys(fingerprint, slot string) error {
	if fingerprint == "" || !slotPattern.MatchString(slot) {
		return ErrInvalidKey
	}
	return nil
}
