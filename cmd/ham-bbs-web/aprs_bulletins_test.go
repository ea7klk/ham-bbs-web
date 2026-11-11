package main

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func seedBulletins(t *testing.T, s *server) {
	t.Helper()
	at := now()
	rows := []dbAPRSBulletin{
		{DedupeKey: "a", FirstSeen: at, LastSeen: at, SeenCount: 3, From: "NWS", Slot: 1, Group: "NWS", Category: "weather", Text: "Tornado warning until 21:00Z"},
		{DedupeKey: "b", FirstSeen: at, LastSeen: at, SeenCount: 1, From: "EA7KLK", Slot: 2, Group: "", Category: "general", Text: "Club meeting moved"},
		{DedupeKey: "c", FirstSeen: at, LastSeen: at, SeenCount: 9, From: "ARRL", Slot: 0, Group: "ARRL", Category: "organisation", Text: "Rule change in force"},
	}
	for i := range rows {
		if err := s.db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
}

// The index shows the categories that hold something, and only those.
func TestAPRSBulletinIndexListsOnlyFilledCategories(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid"})
	seedBulletins(t, s)

	body := invokeUserHandler(s.aprsBulletinIndex, formRequest(http.MethodGet, "/aprs/bulletins", nil), user).Body.String()
	for _, category := range []string{"weather", "organisation", "general"} {
		if !strings.Contains(body, "/aprs/bulletins/"+category) {
			t.Fatalf("the index omits the %s category, which has bulletins", category)
		}
	}
	for _, category := range []string{"emergency", "satellite", "test"} {
		if strings.Contains(body, "/aprs/bulletins/"+category) {
			t.Fatalf("the index shows the empty %s category", category)
		}
	}
}

// An empty table explains itself rather than showing a blank page.
func TestAPRSBulletinIndexEmptyState(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid"})

	body := invokeUserHandler(s.aprsBulletinIndex, formRequest(http.MethodGet, "/aprs/bulletins", nil), user).Body.String()
	if !strings.Contains(body, "aprs_bulletin_none") && !strings.Contains(body, "No bulletins have been received") {
		t.Fatalf("no empty-state message: %q", body)
	}
}

func TestAPRSBulletinListAndDetail(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid"})
	seedBulletins(t, s)

	list := invokeUserHandler(s.aprsBulletinCategory, setPathValue(formRequest(http.MethodGet, "/aprs/bulletins/weather", nil), "category", "weather"), user)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Tornado warning") {
		t.Fatalf("weather list = %d %q", list.Code, list.Body.String())
	}
	if strings.Contains(list.Body.String(), "Club meeting moved") {
		t.Fatal("the weather list shows a bulletin from another category")
	}

	var item dbAPRSBulletin
	if err := s.db.First(&item, "category = ?", "weather").Error; err != nil {
		t.Fatal(err)
	}
	detail := invokeUserHandler(s.aprsBulletinDetail,
		setPathValue(setPathValue(formRequest(http.MethodGet, "/aprs/bulletins/weather/1", nil), "category", "weather"), "id", strconv.Itoa(int(item.ID))), user)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), "Tornado warning") {
		t.Fatalf("detail = %d", detail.Code)
	}
	if !strings.Contains(detail.Body.String(), "NWS") {
		t.Fatal("the detail omits the group, which is how a reader judges the source")
	}

	// A bulletin addressed under the wrong category is a 404, not a redirect: the
	// URL is not the identity, the row is.
	wrong := invokeUserHandler(s.aprsBulletinDetail,
		setPathValue(setPathValue(formRequest(http.MethodGet, "/aprs/bulletins/nets/1", nil), "category", "nets"), "id", strconv.Itoa(int(item.ID))), user)
	if wrong.Code != http.StatusNotFound {
		t.Fatalf("a bulletin under the wrong category = %d, want 404", wrong.Code)
	}
}

// Bulletins are read-only here, and available to any signed-in caller: EnableAPRS
// governs sending, not reading.
func TestAPRSBulletinsReadableWithoutAPRSEnabled(t *testing.T) {
	s := newTestServer(t)
	testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", EnableAPRS: false})
	seedBulletins(t, s)

	// A fresh struct with the flag off, as a request would load it.
	user := &dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: false}
	response := invokeUserHandler(s.aprsBulletinIndex, formRequest(http.MethodGet, "/aprs/bulletins", nil), user)
	if response.Code != http.StatusOK {
		t.Fatalf("bulletins are unreadable with APRS off: %d", response.Code)
	}
}

// Rows older than the retention window are not shown, matching the receiver's own
// pruning so the two cannot disagree about what is live.
func TestAPRSBulletinsRespectTheRetentionWindow(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid"})
	old := time.Now().UTC().Add(-aprsBulletinRetention - time.Hour).Format(aprsBulletinStamp)
	if err := s.db.Create(&dbAPRSBulletin{
		DedupeKey: "old", FirstSeen: old, LastSeen: old, SeenCount: 1,
		From: "NWS", Group: "NWS", Category: "weather", Text: "Long expired",
	}).Error; err != nil {
		t.Fatal(err)
	}
	body := invokeUserHandler(s.aprsBulletinIndex, formRequest(http.MethodGet, "/aprs/bulletins", nil), user).Body.String()
	if strings.Contains(body, "/aprs/bulletins/weather") {
		t.Fatal("an expired bulletin is still listed")
	}
}

// Third-party content is cleaned on read, like every other string this app shows.
func TestAPRSBulletinTextIsSanitisedOnRead(t *testing.T) {
	s := newTestServer(t)
	at := now()
	if err := s.db.Create(&dbAPRSBulletin{
		DedupeKey: "x", FirstSeen: at, LastSeen: at, SeenCount: 1,
		From: "NWS\x1b[2J", Group: "NWS", Category: "weather", Text: "hi\x1b[2Jthere\u202e",
	}).Error; err != nil {
		t.Fatal(err)
	}
	rows, err := s.loadAPRSBulletins("weather", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("loaded %d rows", len(rows))
	}
	if strings.ContainsRune(rows[0].Text, '\x1b') || strings.ContainsRune(rows[0].Text, '\u202e') {
		t.Fatalf("a bulletin reached the page with a terminal sequence: %q", rows[0].Text)
	}
	if strings.ContainsRune(rows[0].From, '\x1b') {
		t.Fatalf("the sender kept a terminal sequence: %q", rows[0].From)
	}
}

func TestBulletinAge(t *testing.T) {
	at := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		at.Format(aprsBulletinStamp):                       "now",
		at.Add(-7 * time.Minute).Format(aprsBulletinStamp): "7m",
		at.Add(-3 * time.Hour).Format(aprsBulletinStamp):   "3h",
		at.Add(-50 * time.Hour).Format(aprsBulletinStamp):  "2d",
		"not a timestamp": "",
	}
	for stamp, want := range cases {
		if got := bulletinAge(stamp, at); got != want {
			t.Fatalf("bulletinAge(%q) = %q, want %q", stamp, got, want)
		}
	}
	// A clock skew must not read as an age from the future.
	future := at.Add(time.Hour).Format(aprsBulletinStamp)
	if got := bulletinAge(future, at); got != "now" {
		t.Fatalf("a future stamp read as %q", got)
	}
}
