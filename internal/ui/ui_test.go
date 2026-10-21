package ui_test

import (
	"ainfra/internal/ui"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUIRequiresSessionAndCSRF(t *testing.T) {
	u := ui.New("http://127.0.0.1:1", http.DefaultClient, false, time.Now)
	w := httptest.NewRecorder()
	u.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 303 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	u.ServeHTTP(w, httptest.NewRequest("GET", "/login", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Вход") {
		t.Fatal(w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal(cookies)
	}
	w = httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/login", strings.NewReader("token=secret"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	u.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal(w.Code)
	}
}
