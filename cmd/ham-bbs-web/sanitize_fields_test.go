package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBulletinBoardAndProfileTextIsSanitised(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid"})

	bulletin := invokeUserHandler(s.bulletinCreate, formRequest(http.MethodPost, "/bulletins/new", url.Values{
		"title": {"Net \x1b[2Jnotes"},
		"body":  {"text\x1b[31mred"},
	}), user)
	assertRedirect(t, bulletin, "/bulletins")
	var stored dbBulletin
	if err := s.db.First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Title != "Net [2Jnotes" || stored.Body != "text[31mred" {
		t.Fatalf("stored bulletin = %#v", stored)
	}

	board := invokeUserHandler(s.boardCreate, formRequest(http.MethodPost, "/boards/new", url.Values{
		"name":        {"APRS \x1b[2JTraffic"},
		"description": {"desc\x1b]52;c;eA==\x07"},
	}), user)
	assertRedirect(t, board, "/boards/aprs-2jtraffic")
	var storedBoard dbBoard
	if err := s.db.First(&storedBoard, "id = ?", "aprs-2jtraffic").Error; err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(storedBoard.Name, '\x1b') || strings.ContainsRune(storedBoard.Description, '\x1b') {
		t.Fatalf("board kept escapes: %#v", storedBoard)
	}

	profile := url.Values{
		"full_name":       {"Volker \x1b[2J"},
		"email":           {"v@example.com"},
		"qth":             {"Home\x1b]52;c;eA==\x07"},
		"rig":             {"HT\u202e"},
		"language":        {"en"},
		"new_password":    {"correct-horse-battery"},
		"verify_password": {"correct-horse-battery"},
	}
	assertRedirect(t, invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", profile), user), "/profile?msg=web_profile_saved")
	var updated dbUser
	if err := s.db.First(&updated, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if updated.FullName != "Volker [2J" || strings.ContainsRune(updated.QTH, '\x1b') || strings.ContainsRune(updated.Rig, '\u202e') {
		t.Fatalf("profile kept escapes: %#v", updated)
	}
}
