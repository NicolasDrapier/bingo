package store

import (
	"database/sql"
	"fmt"
)

var migrations = []struct {
	Version int
	SQL     string
}{
	{1, `
CREATE TABLE games (
  id              INTEGER PRIMARY KEY,
  code            TEXT UNIQUE NOT NULL,
  name            TEXT NOT NULL,
  grid_size       INTEGER NOT NULL CHECK (grid_size IN (3,4,5)),
  free_center     INTEGER NOT NULL DEFAULT 0,
  status          TEXT NOT NULL DEFAULT 'lobby' CHECK (status IN ('lobby','playing','finished')),
  host_player_id  INTEGER REFERENCES players(id),
  created_at      TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE players (
  id          INTEGER PRIMARY KEY,
  game_id     INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  name        TEXT NOT NULL,
  token_hash  TEXT NOT NULL UNIQUE,
  ready       INTEGER NOT NULL DEFAULT 0,
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (game_id, name)
);
CREATE TABLE cells (
  id          INTEGER PRIMARY KEY,
  player_id   INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  row         INTEGER NOT NULL,
  col         INTEGER NOT NULL,
  label       TEXT NOT NULL,
  checked     INTEGER NOT NULL DEFAULT 0,
  checked_at  TEXT,
  UNIQUE (player_id, row, col)
);
CREATE TABLE wins (
  id          INTEGER PRIMARY KEY,
  game_id     INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  player_id   INTEGER NOT NULL REFERENCES players(id) ON DELETE CASCADE,
  pattern     TEXT NOT NULL,
  created_at  TEXT NOT NULL DEFAULT (datetime('now')),
  UNIQUE (game_id, player_id, pattern)
);
CREATE INDEX idx_players_game ON players(game_id);
CREATE INDEX idx_cells_player ON cells(player_id);
CREATE INDEX idx_wins_game ON wins(game_id);
`},
	{2, `CREATE TABLE proposals (
 id INTEGER PRIMARY KEY,
 game_id INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
 player_id INTEGER NOT NULL REFERENCES players(id),
 label TEXT NOT NULL,
 normalized TEXT NOT NULL,
 approved INTEGER NOT NULL DEFAULT 0,
 UNIQUE(game_id, normalized)
);`},
}

func migrate(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("schema_migrations: %w", err)
	}
	for _, m := range migrations {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version=?`, m.Version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", m.Version, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, m.Version); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
