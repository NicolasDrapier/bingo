package http

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestLeaveSessionClearsCookie(t *testing.T) {
	handler := New(nil).Routes(nil)
	jar, _ := cookiejar.New(nil)
	origin, _ := url.Parse("http://bingo.local/")
	jar.SetCookies(origin, []*http.Cookie{{Name: cookieName, Value: "old-session", Path: "/"}})
	request := httptest.NewRequest("POST", "http://bingo.local/api/session/leave", nil)
	for _, c := range jar.Cookies(origin) {
		request.AddCookie(c)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieName || cookies[0].Path != "/" || cookies[0].MaxAge != -1 || !cookies[0].HttpOnly {
		t.Fatalf("deletion cookie: %+v", cookies)
	}
	jar.SetCookies(origin, cookies)
	if len(jar.Cookies(origin)) != 0 {
		t.Fatal("session cookie still present on return to home")
	}
}
