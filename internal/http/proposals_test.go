package http

import (
	"bingo/internal/store"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestProposalAPI(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	g, _, err := st.CreateGame("Test", 3, false, "Host", "host")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = st.JoinGame(g.Code, "Guest", "guest"); err != nil {
		t.Fatal(err)
	}
	handler := New(st).Routes(nil)
	request := func(method, path, token string, body any, want int) *httptest.ResponseRecorder {
		t.Helper()
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d want %d: %s", method, path, w.Code, want, w.Body)
		}
		return w
	}
	request("POST", "/api/proposals", "", map[string]any{"label": "hello"}, 401)
	request("POST", "/api/proposals", "guest", map[string]any{"label": "   "}, 400)
	request("POST", "/api/start", "host", nil, 409)
	for i := 0; i < 9; i++ {
		request("POST", "/api/proposals", "guest", map[string]any{"label": fmt.Sprintf("Item %d", i)}, 201)
	}
	request("POST", "/api/proposals", "host", map[string]any{"label": " item  0 "}, 409)
	request("POST", "/api/start", "guest", nil, 403)
	request("POST", "/api/start", "host", nil, 200)
	for _, token := range []string{"host", "guest"} {
		w := request("GET", "/api/state", token, nil, 200)
		var state statePayload
		if err = json.Unmarshal(w.Body.Bytes(), &state); err != nil {
			t.Fatal(err)
		}
		if state.Game.Status != "playing" || len(state.Grid) != 9 || len(state.Proposals) != 9 {
			t.Fatalf("state: %+v", state)
		}
	}
	request("POST", "/api/proposals", "guest", map[string]any{"label": "late"}, 409)
	request("POST", "/api/start", "host", nil, 409)
}
