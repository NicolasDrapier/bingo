package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbeddedComicAssets(t *testing.T) {
	handler := http.FileServerFS(FS)
	for _, name := range []string{"mascot-welcome.png", "mascot-waiting.png", "mascot-victory.png", "bingo-burst.png"} {
		r := httptest.NewRequest("GET", "/assets/"+name, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || w.Body.Len() == 0 {
			t.Fatalf("asset %s: status %d, type %s", name, w.Code, w.Header().Get("Content-Type"))
		}
	}
}
