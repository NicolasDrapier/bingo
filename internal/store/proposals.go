package store

import (
	"database/sql"
	"errors"
	"math/rand/v2"
	"strings"
)

type Proposal struct {
	ID         int64  `json:"id"`
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	Label      string `json:"label"`
}

var ErrLobbyClosed = errors.New("les propositions sont fermées")
var ErrNotEnoughProposals = errors.New("pas assez de propositions pour remplir la grille")

func (s *Store) ListProposals(gameID int64) ([]Proposal, error) {
	rows, err := s.db.Query(`SELECT q.id,q.player_id,p.name,q.label FROM proposals q JOIN players p ON p.id=q.player_id WHERE q.game_id=? ORDER BY q.id`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Proposal{}
	for rows.Next() {
		var p Proposal
		if err := rows.Scan(&p.ID, &p.PlayerID, &p.PlayerName, &p.Label); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) AddProposal(gameID, playerID int64, label string) error {
	normalized := strings.ToLower(strings.Join(strings.Fields(label), " "))
	res, err := s.db.Exec(`INSERT INTO proposals(game_id,player_id,label,normalized) SELECT id,?,?,? FROM games WHERE id=? AND status='lobby'`, playerID, label, normalized, gameID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrLobbyClosed
	}
	return nil
}

func (s *Store) StartWithRandomGrids(gameID int64) error {
	return s.Tx(func(tx *sql.Tx) error {
		var size int
		var free bool

		res, err := tx.Exec(`UPDATE games SET status='playing' WHERE id=? AND status='lobby'`, gameID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return ErrLobbyClosed
		}
		if err = tx.QueryRow(`SELECT grid_size,free_center FROM games WHERE id=?`, gameID).Scan(&size, &free); err != nil {
			return err
		}
		rows, err := tx.Query(`SELECT label FROM proposals WHERE game_id=? ORDER BY id`, gameID)
		if err != nil {
			return err
		}
		labels := []string{}
		for rows.Next() {
			var label string
			if err = rows.Scan(&label); err != nil {
				rows.Close()
				return err
			}
			labels = append(labels, label)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(labels) < size*size {
			return ErrNotEnoughProposals
		}
		rows, err = tx.Query(`SELECT id FROM players WHERE game_id=? ORDER BY id`, gameID)
		if err != nil {
			return err
		}
		players := []int64{}
		for rows.Next() {
			var id int64
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			players = append(players, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range players {
			if _, err = tx.Exec(`DELETE FROM cells WHERE player_id=?`, id); err != nil {
				return err
			}
			shuffled := append([]string(nil), labels...)
			rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
			cursor := 0
			for i := 0; i < size*size; i++ {
				label := ""
				checked := false
				if free && size%2 == 1 && i == size*size/2 {
					label = "GRATUIT"
					checked = true
				} else {
					label = shuffled[cursor]
					cursor++
				}
				if _, err = tx.Exec(`INSERT INTO cells(player_id,row,col,label,checked) VALUES(?,?,?,?,?)`, id, i/size, i%size, label, checked); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
