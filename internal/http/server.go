package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"bingo/internal/game"
	"bingo/internal/store"

	"github.com/coder/websocket"
)

const cookieName = "token"

type Server struct {
	store  *store.Store
	hubsMu sync.Mutex
	hubs   map[int64]*Hub
}

func New(st *store.Store) *Server {
	return &Server{store: st, hubs: map[int64]*Hub{}}
}

func (s *Server) Routes(static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/games", s.createGame)
	mux.HandleFunc("POST /api/session/leave", s.leaveSession)
	mux.HandleFunc("POST /api/games/{code}/join", s.joinGame)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("POST /api/proposals", s.addProposal)
	mux.HandleFunc("POST /api/start", s.start)
	mux.HandleFunc("POST /api/finish", s.finish)
	mux.HandleFunc("POST /api/cells/toggle", s.toggle)
	mux.HandleFunc("GET /ws", s.ws)
	if static != nil {
		mux.Handle("/", static)
	}
	return mux
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}

func setTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   30 * 24 * 3600,
	})
}

func (s *Server) leaveSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
		Expires: time.Unix(0, 0),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) auth(r *http.Request) (*store.Player, *store.Game, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil, nil, errors.New("non authentifié")
	}
	p, err := s.store.GetPlayerByToken(c.Value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, errors.New("session inconnue")
		}
		return nil, nil, err
	}
	g, err := s.store.GetGame(p.GameID)
	if err != nil {
		return nil, nil, err
	}
	return p, g, nil
}

func (s *Server) hubFor(gameID int64) *Hub {
	s.hubsMu.Lock()
	defer s.hubsMu.Unlock()
	h, ok := s.hubs[gameID]
	if !ok {
		h = newHub()
		s.hubs[gameID] = h
	}
	return h
}

type playerInfo struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Ready bool   `json:"ready"`
	Host  bool   `json:"host"`
}

type winInfo struct {
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	Pattern    string `json:"pattern"`
	Count      int    `json:"count"`
}

type statePayload struct {
	Proposals []store.Proposal `json:"proposals"`
	Game      struct {
		Code       string `json:"code"`
		Name       string `json:"name"`
		GridSize   int    `json:"grid_size"`
		FreeCenter bool   `json:"free_center"`
		Status     string `json:"status"`
		HostID     *int64 `json:"host_player_id"`
	} `json:"game"`
	Players []playerInfo `json:"players"`
	You     struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"you"`
	Grid    []string  `json:"grid,omitempty"`
	Checked []bool    `json:"checked,omitempty"`
	Wins    []winInfo `json:"wins"`
}

func (s *Server) snapshot(g *store.Game, p *store.Player) (*statePayload, error) {
	sp := &statePayload{}
	sp.Game.Code = g.Code
	sp.Game.Name = g.Name
	sp.Game.GridSize = g.GridSize
	sp.Game.FreeCenter = g.FreeCenter
	sp.Game.Status = g.Status
	sp.Game.HostID = g.HostPlayerID
	sp.You.ID = p.ID
	sp.You.Name = p.Name

	players, err := s.store.ListPlayers(g.ID)
	if err != nil {
		return nil, err
	}
	for _, pl := range players {
		sp.Players = append(sp.Players, playerInfo{ID: pl.ID, Name: pl.Name, Ready: pl.Ready, Host: g.HostPlayerID != nil && *g.HostPlayerID == pl.ID})
	}

	if labels, checked, err := s.store.GetGrid(p.ID, g.GridSize); err == nil && labels != nil {
		for r := 0; r < g.GridSize; r++ {
			for c := 0; c < g.GridSize; c++ {
				sp.Grid = append(sp.Grid, labels[r][c])
				sp.Checked = append(sp.Checked, checked[r][c])
			}
		}
	}

	proposals, err := s.store.ListProposals(g.ID)
	if err != nil {
		return nil, err
	}
	sp.Proposals = proposals
	wins, err := s.store.ListWins(g.ID)
	if err != nil {
		return nil, err
	}
	for _, w := range wins {
		sp.Wins = append(sp.Wins, winInfo{PlayerID: w.PlayerID, PlayerName: w.PlayerName, Pattern: w.Pattern, Count: w.Count})
	}
	return sp, nil
}

func validName(s string) bool {
	n := utf8.RuneCountInString(s)
	return n >= 1 && n <= 20
}

func (s *Server) createGame(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name       string `json:"name"`
		PlayerName string `json:"player_name"`
		GridSize   int    `json:"grid_size"`
		FreeCenter bool   `json:"free_center"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "JSON invalide")
		return
	}
	if !validName(in.Name) {
		writeErr(w, 400, "nom de partie : 1 à 20 caractères")
		return
	}
	if !validName(in.PlayerName) {
		writeErr(w, 400, "pseudo : 1 à 20 caractères")
		return
	}
	size := game.Size(in.GridSize)
	if !size.Valid() {
		writeErr(w, 400, "taille de grille : 3, 4 ou 5")
		return
	}
	if in.FreeCenter && !size.HasFreeCenter() {
		writeErr(w, 400, "case gratuite indisponible en 4×4")
		return
	}
	token, err := store.GenToken()
	if err != nil {
		writeErr(w, 500, "erreur interne")
		return
	}
	g, _, err := s.store.CreateGame(in.Name, in.GridSize, in.FreeCenter, in.PlayerName, token)
	if err != nil {
		writeErr(w, 500, "création impossible : "+err.Error())
		return
	}
	setTokenCookie(w, token)
	writeJSON(w, 201, map[string]string{"code": g.Code, "url": "/?code=" + g.Code})
}

func (s *Server) joinGame(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	var in struct {
		PlayerName string `json:"player_name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, 400, "JSON invalide")
		return
	}
	if !validName(in.PlayerName) {
		writeErr(w, 400, "pseudo : 1 à 20 caractères")
		return
	}
	token, err := store.GenToken()
	if err != nil {
		writeErr(w, 500, "erreur interne")
		return
	}
	g, p, err := s.store.JoinGame(code, in.PlayerName, token)
	if err != nil {
		if errors.Is(err, store.ErrGameNotFound) {
			writeErr(w, 404, "partie introuvable")
			return
		}
		if store.IsUniqueViolation(err) {
			writeErr(w, 409, "ce pseudo est déjà pris dans cette partie")
			return
		}
		writeErr(w, 500, "rejoindre impossible : "+err.Error())
		return
	}
	s.hubFor(g.ID).broadcast(encodeMsg("player_joined", map[string]any{
		"player_id": p.ID, "name": p.Name,
	}))
	setTokenCookie(w, token)
	writeJSON(w, 200, map[string]string{"code": code})
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	p, g, err := s.auth(r)
	if err != nil {
		writeErr(w, 401, "non authentifié")
		return
	}
	sp, err := s.snapshot(g, p)
	if err != nil {
		writeErr(w, 500, "état indisponible")
		return
	}
	writeJSON(w, 200, sp)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	p, g, err := s.auth(r)
	if err != nil {
		writeErr(w, 401, "non authentifié")
		return
	}
	if g.HostPlayerID == nil || *g.HostPlayerID != p.ID {
		writeErr(w, 403, "seul l'hôte peut lancer la partie")
		return
	}
	if g.Status != store.StatusLobby {
		writeErr(w, 409, "la partie n'est plus en lobby")
		return
	}
	if err := s.store.StartWithRandomGrids(g.ID); err != nil {
		if errors.Is(err, store.ErrNotEnoughProposals) || errors.Is(err, store.ErrLobbyClosed) {
			writeErr(w, 409, err.Error())
		} else {
			writeErr(w, 500, "distribution impossible")
		}
		return
	}

	s.hubFor(g.ID).broadcast(encodeMsg("game_started", map[string]any{"code": g.Code}))
	writeJSON(w, 200, map[string]string{"status": store.StatusPlaying})
}

func (s *Server) finish(w http.ResponseWriter, r *http.Request) {
	p, g, err := s.auth(r)
	if err != nil {
		writeErr(w, 401, "non authentifié")
		return
	}
	if g.HostPlayerID == nil || *g.HostPlayerID != p.ID {
		writeErr(w, 403, "seul l'hôte peut terminer la partie")
		return
	}
	if g.Status != store.StatusPlaying {
		writeErr(w, 409, "la partie n'est pas en cours")
		return
	}
	if err := s.store.SetGameStatus(g.ID, store.StatusFinished); err != nil {
		writeErr(w, 500, "erreur interne")
		return
	}

	payload, err := s.finalPayload(g)
	if err != nil {
		writeErr(w, 500, "erreur interne")
		return
	}
	s.hubFor(g.ID).broadcast(encodeMsg("game_finished", payload))
	writeJSON(w, 200, payload)
}

func (s *Server) finalPayload(g *store.Game) (map[string]any, error) {
	players, err := s.store.ListPlayers(g.ID)
	if err != nil {
		return nil, err
	}
	wins, err := s.store.ListWins(g.ID)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int{}
	for _, w := range wins {
		counts[w.PlayerID]++
	}
	out := make([]map[string]any, 0, len(players))
	for _, pl := range players {
		entry := map[string]any{"id": pl.ID, "name": pl.Name, "wins": counts[pl.ID]}
		if labels, checked, err := s.store.GetGrid(pl.ID, g.GridSize); err == nil && labels != nil {
			entry["grid"] = labels
			entry["checked"] = checked
		}
		out = append(out, entry)
	}

	sortPlayers(out)
	return map[string]any{
		"code":    g.Code,
		"players": out,
	}, nil
}

func sortPlayers(players []map[string]any) {
	sort.SliceStable(players, func(i, j int) bool {
		a, _ := players[i]["wins"].(int)
		b, _ := players[j]["wins"].(int)
		return a > b
	})
}

func (s *Server) toggle(w http.ResponseWriter, r *http.Request) {
	p, g, err := s.auth(r)
	if err != nil {
		writeErr(w, 401, "non authentifié")
		return
	}
	if g.Status != store.StatusPlaying {
		writeErr(w, 409, "la partie n'est pas en cours")
		return
	}
	var raw struct {
		Row int `json:"row"`
		Col int `json:"col"`
	}
	if err := decode(r, &raw); err != nil {
		writeErr(w, 400, "JSON invalide")
		return
	}
	size := g.GridSize
	if raw.Row < 0 || raw.Row >= size || raw.Col < 0 || raw.Col >= size {
		writeErr(w, 400, "case hors grille")
		return
	}
	if g.FreeCenter {
		if cr, cc, ok := game.Size(size).Center(); ok && raw.Row == cr && raw.Col == cc {
			writeErr(w, 400, "la case gratuite est déjà cochée")
			return
		}
	}
	checked, err := s.store.ToggleCell(p.ID, raw.Row, raw.Col)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	hub := s.hubFor(g.ID)
	hub.broadcast(encodeMsg("cell_toggled", map[string]any{
		"player_id": p.ID, "row": raw.Row, "col": raw.Col, "checked": checked,
	}))

	_, checkedGrid, err := s.store.GetGrid(p.ID, size)
	if err != nil || checkedGrid == nil {
		writeErr(w, 500, "grille indisponible")
		return
	}
	known, err := s.store.KnownPatterns(g.ID, p.ID)
	if err != nil {
		writeErr(w, 500, "erreur interne")
		return
	}
	count := len(known)
	for _, pat := range game.Size(size).Wins(checkedGrid, g.FreeCenter) {
		if known[pat] {
			continue
		}
		added, err := s.store.AddWin(g.ID, p.ID, pat)
		if err != nil {
			writeErr(w, 500, "erreur interne")
			return
		}
		if !added {
			continue
		}
		count++
		hub.broadcast(encodeMsg("bingo", map[string]any{
			"player_id": p.ID, "player_name": p.Name, "pattern": pat, "count": count,
		}))
	}
	writeJSON(w, 200, map[string]bool{"checked": checked})
}

func (s *Server) ws(w http.ResponseWriter, r *http.Request) {
	p, g, err := s.auth(r)
	if err != nil {
		writeErr(w, 401, "non authentifié")
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	c := &client{conn: conn}
	hub := s.hubFor(g.ID)
	hub.add(c)
	defer hub.remove(c)

	sp, err := s.snapshot(g, p)
	if err == nil {
		c.wmu.Lock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn.Write(ctx, websocket.MessageText, encodeMsg("state", sp))
		cancel()
		c.wmu.Unlock()
	}

	for {
		if _, _, err := conn.Read(context.Background()); err != nil {
			return
		}
	}
}
