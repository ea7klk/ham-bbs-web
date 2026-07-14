package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestLanguagePostPersistsSelectedLanguage(t *testing.T) {
	db, err := openDatabase(filepath.Join(t.TempDir(), "bbs.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	user := &dbUser{Callsign: "EA7KLK", FullName: "Test User", Email: "test@example.invalid", Language: "en"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	s := &server{db: db, cfg: config{bbsLog: filepath.Join(t.TempDir(), "bbs.log")}}
	form := url.Values{"language": {"es"}, "return_to": {"/aprs"}}
	req := httptest.NewRequest(http.MethodPost, "/language", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	s.languagePost(response, req, user)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("languagePost() status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if location := response.Header().Get("Location"); location != "/aprs" {
		t.Fatalf("languagePost() location = %q, want %q", location, "/aprs")
	}
	var saved dbUser
	if err := db.First(&saved, "callsign = ?", user.Callsign).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Language != "es" {
		t.Fatalf("saved language = %q, want %q", saved.Language, "es")
	}
}
