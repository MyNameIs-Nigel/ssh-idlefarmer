package store

import (
	"context"
	"fmt"
)

// migrations are applied in order on startup. The schema version lives in
// SQLite's user_version pragma; each step runs inside a transaction together
// with the version bump, so a crash mid-migration leaves the database at the
// previous version. Steps must be forward-only, ordered, and never
// destructive to existing player data.
var migrations = []string{
	// v1 (Task 2): accounts by key fingerprint, saves by (fingerprint, slot).
	// The save's game state is a versioned JSON payload owned by internal/sim.
	`CREATE TABLE accounts (
		fingerprint TEXT PRIMARY KEY,
		public_key  TEXT NOT NULL,
		first_seen  INTEGER NOT NULL,
		last_seen   INTEGER NOT NULL
	);
	CREATE TABLE saves (
		fingerprint TEXT NOT NULL REFERENCES accounts(fingerprint),
		slot        TEXT NOT NULL,
		created_at  INTEGER NOT NULL,
		last_active INTEGER NOT NULL,
		state       BLOB NOT NULL,
		PRIMARY KEY (fingerprint, slot)
	) WITHOUT ROWID;`,

	// v2 (Task 4): progression added prestige/unlock/achievement fields to
	// the save payload, bumping the payload to version 2. This column
	// surfaces each row's payload version for operators; existing rows are
	// version-1 payloads, which internal/sim upgrades in place on load.
	`ALTER TABLE saves ADD COLUMN state_version INTEGER NOT NULL DEFAULT 1;`,
}

func (st *Store) migrate(ctx context.Context) error {
	var current int
	if err := st.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return fmt.Errorf("store: read schema version: %w", err)
	}
	if current > len(migrations) {
		return fmt.Errorf("store: database schema version %d is newer than supported %d", current, len(migrations))
	}

	for v := current; v < len(migrations); v++ {
		tx, err := st.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: begin migration %d: %w", v+1, err)
		}
		if _, err := tx.ExecContext(ctx, migrations[v]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: apply migration %d: %w", v+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v+1)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: bump schema version to %d: %w", v+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit migration %d: %w", v+1, err)
		}
	}
	return nil
}

// SchemaVersion returns the database's current schema version.
func (st *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	err := st.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}
