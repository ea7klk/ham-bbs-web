package main

import (
	"net/http"
	"strings"
	"testing"
)

// The dashboard leads with the newest bulletin.
//
// "Newest" is not "first row": a bulletin takes the next position when it is
// published, so loadBulletins returns them oldest-first and the newest is the
// last one. The first version of the dashboard listed the three oldest and
// called them latest, which is the bug these tests exist to prevent.

func seedBulletinsInOrder(t *testing.T, s *server, titles ...string) {
	t.Helper()
	for i, title := range titles {
		if err := s.db.Create(&dbBulletin{
			Position: i, Title: title, Body: "Bulletin body text.", Updated: now(), From: "EA7KLK",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
}

func homeBody(t *testing.T, s *server, user *dbUser) string {
	t.Helper()
	response := invokeUserHandler(s.home, formRequest(http.MethodGet, "/", nil), user)
	if response.Code != http.StatusOK {
		t.Fatalf("home status = %d", response.Code)
	}
	return response.Body.String()
}

func TestDashboardPinsTheNewestBulletin(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "op@example.invalid"})
	seedBulletinsInOrder(t, s, "Oldest", "Middle", "Newest")

	body := homeBody(t, s, user)
	pinned := pinnedBulletinTitle(t, body)
	if pinned != "Newest" {
		t.Fatalf("the pinned bulletin is %q, want the newest", pinned)
	}
}

// pinnedBulletinTitle reads the title out of the pinned block.
func pinnedBulletinTitle(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `class="pinned"`)
	if start < 0 {
		return ""
	}
	block := body[start:]
	end := strings.Index(block, "</section>")
	if end < 0 {
		t.Fatal("the pinned block is not closed")
	}
	block = block[:end]
	i := strings.Index(block, `id="pinned-title">`)
	if i < 0 {
		t.Fatal("the pinned block has no title")
	}
	rest := block[i+len(`id="pinned-title">`):]
	return rest[:strings.Index(rest, "</h2>")]
}

// The pinned bulletin appears once: the list below it holds the ones before it,
// newest first, so nothing is repeated.
func TestDashboardListsTheOlderBulletinsBelow(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "op@example.invalid"})
	seedBulletinsInOrder(t, s, "Oldest", "Middle", "Newest")

	body := homeBody(t, s, user)
	if got := strings.Count(body, "Newest"); got != 1 {
		t.Fatalf("\"Newest\" appears %d times; it should appear once, as the pin", got)
	}
	// The remaining list should read newest-first too.
	iMiddle := strings.Index(body, "Middle")
	iOldest := strings.Index(body, "Oldest")
	if iMiddle < 0 || iOldest < 0 {
		t.Fatal("the older bulletins are not listed")
	}
	if iMiddle > iOldest {
		t.Fatal("the older bulletins are listed oldest-first")
	}
}

// At most three are listed below the pin, so a board with a long history does
// not push everything else off the page.
func TestDashboardCapsTheOlderBulletins(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "op@example.invalid"})
	seedBulletinsInOrder(t, s, "B1", "B2", "B3", "B4", "B5", "B6")

	body := homeBody(t, s, user)
	if pinned := pinnedBulletinTitle(t, body); pinned != "B6" {
		t.Fatalf("pinned = %q, want B6", pinned)
	}
	// B5, B4, B3 listed; B2 and B1 dropped.
	for _, want := range []string{"B5", "B4", "B3"} {
		if !strings.Contains(body, want) {
			t.Fatalf("%q is missing from the dashboard", want)
		}
	}
	for _, dropped := range []string{"B2", "B1"} {
		if strings.Contains(body, dropped) {
			t.Fatalf("%q should be past the cap but is on the page", dropped)
		}
	}
}

// With no bulletins the block is absent rather than an empty box, and the boards
// section takes the full width.
func TestDashboardWithoutBulletins(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "op@example.invalid"})
	if err := s.db.Create(&dbBoard{ID: defaultBoardID, Position: 0, Name: "General", Created: now()}).Error; err != nil {
		t.Fatal(err)
	}

	body := homeBody(t, s, user)
	if strings.Contains(body, `class="pinned"`) {
		t.Fatal("an empty bulletin block is rendered")
	}
	if !strings.Contains(body, `class="split single"`) {
		t.Fatal("the layout did not collapse to one column when there are no older bulletins")
	}
}

// With exactly one bulletin it is pinned and there is no list beneath it.
func TestDashboardWithOneBulletin(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "op@example.invalid"})
	seedBulletinsInOrder(t, s, "Only one")

	body := homeBody(t, s, user)
	if got := pinnedBulletinTitle(t, body); got != "Only one" {
		t.Fatalf("pinned = %q", got)
	}
	if !strings.Contains(body, `class="split single"`) {
		t.Fatal("the aside is shown with nothing to put in it")
	}
	// Its body is shown in full, not only the title: the dashboard is where an
	// announcement is read, not a link to it.
	if !strings.Contains(body, "Bulletin body text.") {
		t.Fatal("the pinned bulletin's body is not shown")
	}
}

// Bulletin text is third-party-ish content that goes through the sanitising rule,
// so the pinned block must carry the cleaned text.
func TestDashboardPinnedBulletinIsSanitised(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "op@example.invalid"})
	if err := s.db.Create(&dbBulletin{
		Position: 0, Title: "Net \x1b[2Jnotes", Body: "hi\x1b[2Jthere\u202e",
		Updated: now(), From: "EA7KLK",
	}).Error; err != nil {
		t.Fatal(err)
	}
	body := homeBody(t, s, user)
	if strings.ContainsRune(body, '\x1b') || strings.ContainsRune(body, '\u202e') {
		t.Fatal("the pinned bulletin carries a terminal sequence")
	}
	if !strings.Contains(body, "Net [2Jnotes") {
		t.Fatal("the pinned title is not the sanitised one")
	}
}
