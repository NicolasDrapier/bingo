package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"

	_ "modernc.org/sqlite"
)

var ErrGameNotFound = errors.New("partie introuvable")

const (
	StatusLobby    = "lobby"
	StatusPlaying  = "playing"
	StatusFinished = "finished"
)

type Game struct {
	ID           int64
	Code         string
	Name         string
	GridSize     int
	FreeCenter   bool
	Status       string
	HostPlayerID *int64
	CreatedAt    string
}

type Player struct {
	ID        int64
	GameID    int64
	Name      string
	TokenHash string
	Ready     bool
	CreatedAt string
}

type Cell struct {
	PlayerID  int64
	Row, Col  int
	Label     string
	Checked   bool
	CheckedAt *string
}

type Win struct {
	ID        int64
	GameID    int64
	PlayerID  int64
	Pattern   string
	CreatedAt string
}

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Tx(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func GenCode() (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, 4)
	for i, c := range b {
		out[i] = alphabet[int(c)%len(alphabet)]
	}
	return string(out), nil
}

func GenToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (s *Store) CreateGame(name string, gridSize int, freeCenter bool, hostName, hostToken string) (*Game, *Player, error) {
	var g *Game
	var p *Player
	err := s.Tx(func(tx *sql.Tx) error {
		for attempt := 0; attempt < 5; attempt++ {
			code, err := GenCode()
			if err != nil {
				return err
			}
			res, err := tx.Exec(`INSERT INTO games (code, name, grid_size, free_center, status) VALUES (?,?,?,?,?)`,
				code, name, gridSize, freeCenter, StatusLobby)
			if IsUniqueViolation(err) {
				continue
			}
			if err != nil {
				return err
			}
			gid, _ := res.LastInsertId()
			pres, err := tx.Exec(`INSERT INTO players (game_id, name, token_hash) VALUES (?,?,?)`,
				gid, hostName, HashToken(hostToken))
			if err != nil {
				return err
			}
			pid, _ := pres.LastInsertId()
			if _, err := tx.Exec(`UPDATE games SET host_player_id=? WHERE id=?`, pid, gid); err != nil {
				return err
			}
			g = &Game{ID: gid, Code: code, Name: name, GridSize: gridSize, FreeCenter: freeCenter, Status: StatusLobby, HostPlayerID: &pid}
			p = &Player{ID: pid, GameID: gid, Name: hostName, TokenHash: HashToken(hostToken)}
			return nil
		}
		return errors.New("collision de codes trop fréquente")
	})
	if err != nil {
		return nil, nil, err
	}
	return g, p, nil
}

func (s *Store) JoinGame(code, name, token string) (*Game, *Player, error) {
	hash := HashToken(token)
	p := &Player{}
	err := s.Tx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO players (game_id, name, token_hash) SELECT id, ?, ? FROM games WHERE code=? AND status='lobby'`,
			name, hash, code)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrGameNotFound
		}
		return tx.QueryRow(`SELECT id, game_id, name, token_hash, ready, created_at FROM players WHERE token_hash=?`, hash).
			Scan(&p.ID, &p.GameID, &p.Name, &p.TokenHash, &p.Ready, &p.CreatedAt)
	})
	if err != nil {
		return nil, nil, err
	}
	g, err := s.GetGame(p.GameID)
	if err != nil {
		return nil, nil, err
	}
	return g, p, nil
}

const gameSQL = `SELECT id, code, name, grid_size, free_center, status, host_player_id, created_at FROM games`

func scanGame(row *sql.Row) (*Game, error) {
	var g Game
	var host *int64
	if err := row.Scan(&g.ID, &g.Code, &g.Name, &g.GridSize, &g.FreeCenter, &g.Status, &host, &g.CreatedAt); err != nil {
		return nil, err
	}
	g.HostPlayerID = host
	return &g, nil
}

func (s *Store) GetGame(id int64) (*Game, error) {
	return scanGame(s.db.QueryRow(gameSQL+` WHERE id=?`, id))
}

func (s *Store) GetGameByCode(code string) (*Game, error) {
	return scanGame(s.db.QueryRow(gameSQL+` WHERE code=?`, code))
}

func (s *Store) SetGameStatus(id int64, status string) error {
	_, err := s.db.Exec(`UPDATE games SET status=? WHERE id=?`, status, id)
	return err
}

func (s *Store) GetPlayerByToken(token string) (*Player, error) {
	row := s.db.QueryRow(`SELECT id, game_id, name, token_hash, ready, created_at FROM players WHERE token_hash=?`, HashToken(token))
	return scanPlayer(row)
}

func scanPlayer(row *sql.Row) (*Player, error) {
	var p Player
	if err := row.Scan(&p.ID, &p.GameID, &p.Name, &p.TokenHash, &p.Ready, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListPlayers(gameID int64) ([]*Player, error) {
	rows, err := s.db.Query(`SELECT id, game_id, name, token_hash, ready, created_at FROM players WHERE game_id=? ORDER BY id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Player
	for rows.Next() {
		p, err := scanPlayerRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanPlayerRows(rows *sql.Rows) (*Player, error) {
	var p Player
	if err := rows.Scan(&p.ID, &p.GameID, &p.Name, &p.TokenHash, &p.Ready, &p.CreatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) SetReady(playerID int64, ready bool) error {
	_, err := s.db.Exec(`UPDATE players SET ready=? WHERE id=?`, ready, playerID)
	return err
}

func (s *Store) UpsertGrid(playerID int64, labels [][]string) error {
	return s.Tx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM cells WHERE player_id=?`, playerID); err != nil {
			return err
		}
		for r, row := range labels {
			for c, label := range row {
				if _, err := tx.Exec(`INSERT INTO cells (player_id, row, col, label) VALUES (?,?,?,?)`,
					playerID, r, c, label); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Store) GetGrid(playerID int64, size int) (labels [][]string, checked [][]bool, err error) {
	rows, err := s.db.Query(`SELECT row, col, label, checked FROM cells WHERE player_id=?`, playerID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	labels = make([][]string, size)
	checked = make([][]bool, size)
	for r := range labels {
		labels[r] = make([]string, size)
		checked[r] = make([]bool, size)
	}
	n := 0
	for rows.Next() {
		var r, c int
		var label string
		var chk bool
		if err := rows.Scan(&r, &c, &label, &chk); err != nil {
			return nil, nil, err
		}
		if r < 0 || r >= size || c < 0 || c >= size {
			continue
		}
		labels[r][c] = label
		checked[r][c] = chk
		n++
	}
	if n == 0 {
		return nil, nil, nil
	}
	return labels, checked, rows.Err()
}

func (s *Store) ToggleCell(playerID int64, row, col int) (bool, error) {
	var checked bool
	err := s.Tx(func(tx *sql.Tx) error {
		res, err := tx.Exec(`UPDATE cells SET checked = 1 - checked, checked_at = CASE WHEN checked THEN NULL ELSE datetime('now') END
			WHERE player_id=? AND row=? AND col=?`, playerID, row, col)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return errors.New("case introuvable")
		}
		return tx.QueryRow(`SELECT checked FROM cells WHERE player_id=? AND row=? AND col=?`, playerID, row, col).Scan(&checked)
	})
	return checked, err
}

func (s *Store) MarkFreeCenters(gameID int64, centerRow, centerCol int) error {
	_, err := s.db.Exec(`UPDATE cells SET checked=1, checked_at=datetime('now')
		WHERE row=? AND col=? AND player_id IN (SELECT id FROM players WHERE game_id=?)`,
		centerRow, centerCol, gameID)
	return err
}

func (s *Store) KnownPatterns(gameID, playerID int64) (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT pattern FROM wins WHERE game_id=? AND player_id=?`, gameID, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out[p] = true
	}
	return out, rows.Err()
}

func (s *Store) AddWin(gameID, playerID int64, pattern string) (bool, error) {
	res, err := s.db.Exec(`INSERT INTO wins (game_id, player_id, pattern) VALUES (?,?,?)`, gameID, playerID, pattern)
	if IsUniqueViolation(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_ = res
	return true, nil
}

func (s *Store) ListWins(gameID int64) ([]WinRow, error) {
	rows, err := s.db.Query(`
		SELECT w.player_id, p.name, w.pattern, w.created_at,
		       (SELECT COUNT(*) FROM wins w2 WHERE w2.game_id=w.game_id AND w2.player_id=w.player_id)
		FROM wins w JOIN players p ON p.id = w.player_id
		WHERE w.game_id=? ORDER BY w.created_at, w.id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WinRow
	for rows.Next() {
		var w WinRow
		if err := rows.Scan(&w.PlayerID, &w.PlayerName, &w.Pattern, &w.CreatedAt, &w.Count); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

type WinRow struct {
	PlayerID   int64
	PlayerName string
	Pattern    string
	CreatedAt  string
	Count      int
}

func IsUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "constraint failed")
}
