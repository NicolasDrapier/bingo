package store

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestProposalDistribution(t *testing.T) {
	for _, size := range []int{3, 4, 5} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			s, err := New(filepath.Join(t.TempDir(), "bingo.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			g, host, err := s.CreateGame("Test", size, size%2 == 1, "Host", "host")
			if err != nil {
				t.Fatal(err)
			}
			_, other, err := s.JoinGame(g.Code, "Other", "other")
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < size*size-1; i++ {
				if err = s.AddProposal(g.ID, other.ID, fmt.Sprintf("Situation %d", i)); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.AddProposal(g.ID, host.ID, "SITUATION 0"); !IsUniqueViolation(err) {
				t.Fatalf("duplicate: %v", err)
			}
			if err = s.StartWithRandomGrids(g.ID); !errors.Is(err, ErrNotEnoughProposals) {
				t.Fatalf("below threshold: %v", err)
			}
			state, _ := s.GetGame(g.ID)
			if state.Status != StatusLobby {
				t.Fatal("failed start must rollback")
			}
			if err = s.AddProposal(g.ID, host.ID, "Dernière proposition"); err != nil {
				t.Fatal(err)
			}
			props, err := s.ListProposals(g.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.StartWithRandomGrids(g.ID); err != nil {
				t.Fatal(err)
			}
			valid := map[string]bool{}
			for _, p := range props {
				valid[p.Label] = true
			}
			for _, player := range []*Player{host, other} {
				grid, checked, err := s.GetGrid(player.ID, size)
				if err != nil {
					t.Fatal(err)
				}
				seen := map[string]bool{}
				for r, row := range grid {
					for c, label := range row {
						if g.FreeCenter && r == size/2 && c == size/2 {
							if label != "GRATUIT" || !checked[r][c] {
								t.Fatal("free center")
							}
							continue
						}
						if !valid[label] || seen[label] || checked[r][c] {
							t.Fatalf("invalid distributed cell: %q", label)
						}
						seen[label] = true
					}
				}
				if len(seen) != size*size-boolInt(g.FreeCenter) {
					t.Fatal("incomplete grid")
				}
			}
			if err = s.StartWithRandomGrids(g.ID); !errors.Is(err, ErrLobbyClosed) {
				t.Fatalf("second start: %v", err)
			}
			if err = s.AddProposal(g.ID, host.ID, "Late"); !errors.Is(err, ErrLobbyClosed) {
				t.Fatalf("late proposal: %v", err)
			}
			if _, _, err = s.JoinGame(g.Code, "Late", "late"); err == nil {
				t.Fatal("late join must not create gridless player")
			}
		})
	}
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
