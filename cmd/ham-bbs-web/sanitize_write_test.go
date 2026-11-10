package main

import (
	"net/http"
	"net/url"
	"testing"
)

func TestPostedMessageCannotCarryTerminalSequences(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid"})
	if err := s.db.Create(&dbBoard{ID: defaultBoardID, Position: 0, Name: "General", Created: now()}).Error; err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"subject": {"hi\x1b[2Jthere"},
		"body":    {"body\x1b]52;c;ZXZpbA==\x07\n\n\n\nend"},
	}
	response := invokeUserHandler(s.messagePost, setPathValue(formRequest(http.MethodPost, "/boards/general/post", form), "id", defaultBoardID), user)
	assertRedirect(t, response, "/boards/"+defaultBoardID)

	var msg dbMessage
	if err := s.db.First(&msg).Error; err != nil {
		t.Fatal(err)
	}
	if msg.Subject != "hi[2Jthere" {
		t.Fatalf("stored subject = %q", msg.Subject)
	}
	if msg.Body != "body]52;c;ZXZpbA==\n\nend" {
		t.Fatalf("stored body = %q", msg.Body)
	}
}
