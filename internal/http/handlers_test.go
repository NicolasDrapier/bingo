package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"bingo/internal/store"

	"github.com/coder/websocket"
)

func testApp(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ts := httptest.NewServer(New(st).Routes(nil))
	t.Cleanup(ts.Close)
	return ts
}

func do(t *testing.T, client *http.Client, method, url string, body, out any) int {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil && res.Body != nil {
			t.Fatalf("décodage %s : %v", url, err)
		}
	}
	return res.StatusCode
}

func clientWithJar(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

type gameOut struct {
	Code string `json:"code"`
}

type stateOut struct {
	Proposals []store.Proposal `json:"proposals"`
	Game      struct {
		Status   string `json:"status"`
		GridSize int    `json:"grid_size"`
		HostID   *int64 `json:"host_player_id"`
	} `json:"game"`
	You struct {
		ID int64 `json:"id"`
	} `json:"you"`
	Players []struct {
		ID    int64 `json:"id"`
		Ready bool  `json:"ready"`
	} `json:"players"`
	Grid    []string  `json:"grid"`
	Checked []bool    `json:"checked"`
	Wins    []winInfo `json:"wins"`
}

func grid3(labels ...string) [][]string {
	if len(labels) != 9 {
		panic("9 labels attendus")
	}
	return [][]string{
		{labels[0], labels[1], labels[2]},
		{labels[3], labels[4], labels[5]},
		{labels[6], labels[7], labels[8]},
	}
}

func TestGameFlow(t *testing.T) {
	ts := testApp(t)
	host := clientWithJar(t)
	other := clientWithJar(t)

	var g gameOut
	code := do(t, host, "POST", ts.URL+"/api/games",
		map[string]any{"name": "Test", "player_name": "Alice", "grid_size": 3, "free_center": false}, &g)
	if code != 201 || len(g.Code) != 4 {
		t.Fatalf("création : statut %d, code %q", code, g.Code)
	}

	if c := do(t, host, "POST", ts.URL+"/api/games",
		map[string]any{"name": "X", "player_name": "B", "grid_size": 6}, nil); c != 400 {
		t.Fatalf("taille 6 attendue en 400, got %d", c)
	}

	if c := do(t, host, "POST", ts.URL+"/api/games",
		map[string]any{"name": "X", "player_name": "B", "grid_size": 4, "free_center": true}, nil); c != 400 {
		t.Fatalf("free_center 4x4 attendu en 400, got %d", c)
	}

	var st stateOut
	if c := do(t, host, "GET", ts.URL+"/api/state", nil, &st); c != 200 {
		t.Fatalf("état : %d", c)
	}
	if st.Game.Status != "lobby" {
		t.Fatalf("statut %q, want lobby", st.Game.Status)
	}

	if c := do(t, host, "POST", ts.URL+"/api/start", nil, nil); c != 409 {
		t.Fatalf("start vide : %d", c)
	}

	var j gameOut
	if c := do(t, other, "POST", ts.URL+"/api/games/"+g.Code+"/join",
		map[string]any{"player_name": "Bob"}, &j); c != 200 {
		t.Fatalf("join : %d", c)
	}

	if c := do(t, clientWithJar(t), "POST", ts.URL+"/api/games/"+g.Code+"/join",
		map[string]any{"player_name": "Alice"}, nil); c != 409 {
		t.Fatalf("pseudo dupliqué attendu en 409, got %d", c)
	}

	if c := do(t, clientWithJar(t), "POST", ts.URL+"/api/games/ZZZZ/join",
		map[string]any{"player_name": "Zoe"}, nil); c != 404 {
		t.Fatalf("code inconnu attendu en 404, got %d", c)
	}
	for _, label := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"} {
		if c := do(t, other, "POST", ts.URL+"/api/proposals", map[string]any{"label": label}, nil); c != 201 {
			t.Fatalf("proposal : %d", c)
		}
	}

	if c := do(t, other, "POST", ts.URL+"/api/start", nil, nil); c != 403 {
		t.Fatalf("start non-hôte attendu en 403, got %d", c)
	}
	if c := do(t, host, "POST", ts.URL+"/api/start", nil, nil); c != 200 {
		t.Fatalf("start : %d", c)
	}

	for _, col := range []int{0, 1, 2} {
		var out map[string]bool
		if c := do(t, host, "POST", ts.URL+"/api/cells/toggle",
			map[string]any{"row": 0, "col": col}, &out); c != 200 || !out["checked"] {
			t.Fatalf("toggle (%d) : %d %v", col, c, out)
		}
	}
	if c := do(t, host, "GET", ts.URL+"/api/state", nil, &st); c != 200 {
		t.Fatalf("état après toggle : %d", c)
	}
	if len(st.Wins) != 1 || st.Wins[0].Pattern != "row:0" {
		t.Fatalf("wins : %+v, want [row:0]", st.Wins)
	}

	if c := do(t, host, "POST", ts.URL+"/api/cells/toggle",
		map[string]any{"row": 5, "col": 0}, nil); c != 400 {
		t.Fatalf("toggle hors grille attendu en 400, got %d", c)
	}

	if c := do(t, other, "POST", ts.URL+"/api/finish", nil, nil); c != 403 {
		t.Fatalf("finish non-hôte attendu en 403, got %d", c)
	}
	var fin map[string]any
	if c := do(t, host, "POST", ts.URL+"/api/finish", nil, &fin); c != 200 {
		t.Fatalf("finish : %d", c)
	}
	players, _ := fin["players"].([]any)
	if len(players) != 2 {
		t.Fatalf("fin : %d joueurs, want 2", len(players))
	}

	if c := do(t, host, "POST", ts.URL+"/api/cells/toggle",
		map[string]any{"row": 1, "col": 1}, nil); c != 409 {
		t.Fatalf("toggle après fin attendu en 409, got %d", c)
	}
}

func TestStateUnauthenticated(t *testing.T) {
	ts := testApp(t)
	res, err := http.Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("état sans cookie : %d, want 401", res.StatusCode)
	}
}

func dialWS(t *testing.T, ts *httptest.Server, client *http.Client) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(ts.URL)
	header := http.Header{}
	for _, c := range client.Jar.Cookies(u) {
		header.Add("Cookie", c.Name+"="+c.Value)
	}
	wsURL := "ws" + ts.URL[len("http"):] + "/ws"
	conn, _, err := websocket.Dial(context.Background(), wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func TestWebSocketBroadcast(t *testing.T) {
	ts := testApp(t)
	host := clientWithJar(t)
	var g gameOut
	do(t, host, "POST", ts.URL+"/api/games",
		map[string]any{"name": "BS", "player_name": "Ann", "grid_size": 3}, &g)

	wsHost := dialWS(t, ts, host)
	defer wsHost.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := wsHost.Read(ctx); err != nil {
		t.Fatal(err)
	}

	other := clientWithJar(t)
	if c := do(t, other, "POST", ts.URL+"/api/games/"+g.Code+"/join",
		map[string]any{"player_name": "Bob"}, nil); c != 200 {
		t.Fatalf("join : %d", c)
	}
	_, data, err := wsHost.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Type    string `json:"type"`
		Payload struct {
			Name string `json:"name"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Type != "player_joined" || m.Payload.Name != "Bob" {
		t.Fatalf("message reçu : %+v, want player_joined/Bob", m)
	}
}

func TestWebSocketState(t *testing.T) {
	ts := testApp(t)
	host := clientWithJar(t)
	var g gameOut
	do(t, host, "POST", ts.URL+"/api/games",
		map[string]any{"name": "WS", "player_name": "Ann", "grid_size": 3}, &g)

	conn := dialWS(t, ts, host)
	defer conn.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Type    string   `json:"type"`
		Payload stateOut `json:"payload"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Type != "state" || m.Payload.Game.Status != "lobby" {
		t.Fatalf("message ws : %+v", m.Type)
	}
}
