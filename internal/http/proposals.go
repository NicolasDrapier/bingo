package http

import (
	"bingo/internal/store"
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Server) addProposal(w http.ResponseWriter, r *http.Request) {
	p, g, err := s.auth(r)
	if err != nil {
		writeErr(w, 401, "non authentifié")
		return
	}
	if g.Status != store.StatusLobby {
		writeErr(w, 409, "les propositions sont fermées")
		return
	}
	var in struct {
		Label string `json:"label"`
	}
	if decode(r, &in) != nil {
		writeErr(w, 400, "JSON invalide")
		return
	}
	label := strings.Join(strings.Fields(in.Label), " ")
	if utf8.RuneCountInString(label) < 1 || utf8.RuneCountInString(label) > 40 {
		writeErr(w, 400, "proposition : 1 à 40 caractères")
		return
	}
	if err = s.store.AddProposal(g.ID, p.ID, label); err != nil {
		if store.IsUniqueViolation(err) {
			writeErr(w, 409, "cette proposition existe déjà")
		} else {
			writeErr(w, 409, err.Error())
		}
		return
	}
	s.hubFor(g.ID).broadcast(encodeMsg("proposals_updated", nil))
	writeJSON(w, 201, map[string]bool{"ok": true})
}
