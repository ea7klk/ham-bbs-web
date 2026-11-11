package main

import (
	"net/http"
	"strings"
	"testing"
)

// APRS message sections.
//
// Bot traffic used to share the list with messages from other operators. These
// tests pin the classification, the SQL that applies it, and that the sections
// partition the history rather than hiding messages.

func TestScopeOfPeer(t *testing.T) {
	cases := map[string]aprsScope{
		"APRSPH":   aprsScopeAPRSPH,
		"aprsph":   aprsScopeAPRSPH,
		"APRSPH-0": aprsScopeAPRSPH,
		"ANSRVR":   aprsScopeThursday,
		"ansrvr":   aprsScopeThursday,
		"DN9RZ":    aprsScopeDirect,
		"DN9RZ-10": aprsScopeDirect,
		"":         aprsScopeDirect,
		"APRSPHX":  aprsScopeDirect,
	}
	for peer, want := range cases {
		if got := aprsScopeOfPeer(peer); got != want {
			t.Fatalf("aprsScopeOfPeer(%q) = %q, want %q", peer, got, want)
		}
	}
}

func TestScopeOfParam(t *testing.T) {
	for value, want := range map[string]aprsScope{
		"":         aprsScopeDirect,
		"direct":   aprsScopeDirect,
		"aprsph":   aprsScopeAPRSPH,
		"thursday": aprsScopeThursday,
	} {
		got, ok := aprsScopeOfParam(value)
		if !ok || got != want {
			t.Fatalf("aprsScopeOfParam(%q) = %q,%t", value, got, ok)
		}
	}
	for _, bad := range []string{"nonsense", "../etc", "APRSPH"} {
		if _, ok := aprsScopeOfParam(bad); ok {
			t.Fatalf("aprsScopeOfParam(%q) accepted an unknown section", bad)
		}
	}
}

// seedScopedAPRS builds a history with traffic in every section, for one user.
func seedScopedAPRS(t *testing.T, s *server) *dbUser {
	t.Helper()
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "op@example.invalid", EnableAPRS: true})

	received := []dbAPRSReceived{
		{UserCallsign: "EA7KLK", Position: 0, At: now(), From: "DN9RZ", To: "EA7KLK", Text: "from a person"},
		{UserCallsign: "EA7KLK", Position: 1, At: now(), From: "EA1ABC-7", To: "EA7KLK", Text: "from another person"},
		{UserCallsign: "EA7KLK", Position: 2, At: now(), From: "APRSPH", To: "EA7KLK", Text: "N:APRSPH 3 Messages Sent"},
		{UserCallsign: "EA7KLK", Position: 3, At: now(), From: "ANSRVR", To: "EA7KLK", Text: "N:HOTG 239 Messages Sent"},
		{UserCallsign: "EA7KLK", Position: 4, At: now(), From: "ANSRVR", To: "EA7KLK", Text: "ack1B}"},
	}
	for i := range received {
		if err := s.db.Create(&received[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	sent := []dbAPRSSent{
		{UserCallsign: "EA7KLK", Position: 0, At: now(), From: "EA7KLK-0", To: "DN9RZ", Text: "hello"},
		{UserCallsign: "EA7KLK", Position: 1, At: now(), From: "EA7KLK-0", To: "APRSPH", Text: "LIST"},
		{UserCallsign: "EA7KLK", Position: 2, At: now(), From: "EA7KLK-0", To: "ANSRVR", Text: "CQ HOTG net"},
	}
	for i := range sent {
		if err := s.db.Create(&sent[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	return user
}

// The SQL filter is the part that could silently disagree with the Go
// classification, so it is checked against the same rows.
func TestScopeWhereMatchesTheClassification(t *testing.T) {
	s := newTestServer(t)
	seedScopedAPRS(t, s)

	cases := map[aprsScope]struct{ received, sent int64 }{
		aprsScopeDirect:   {2, 1},
		aprsScopeAPRSPH:   {1, 1},
		aprsScopeThursday: {2, 1},
	}
	for scope, want := range cases {
		var received, sent int64
		scope.scopeWhere(s.db.Model(&dbAPRSReceived{}), "from").Count(&received)
		scope.scopeWhere(s.db.Model(&dbAPRSSent{}), "to").Count(&sent)
		if received != want.received || sent != want.sent {
			t.Fatalf("%s: received=%d sent=%d, want %d/%d", scope, received, sent, want.received, want.sent)
		}
	}
}

// The sections must partition the history exactly: every message in one, none in
// two, none missing.
func TestScopeWherePartitionsTheHistory(t *testing.T) {
	s := newTestServer(t)
	seedScopedAPRS(t, s)

	var total int64
	s.db.Model(&dbAPRSReceived{}).Count(&total)
	var sum int64
	for _, scope := range aprsScopes {
		var count int64
		scope.scopeWhere(s.db.Model(&dbAPRSReceived{}), "from").Count(&count)
		sum += count
	}
	if sum != total {
		t.Fatalf("sections hold %d of %d received messages", sum, total)
	}

	s.db.Model(&dbAPRSSent{}).Count(&total)
	sum = 0
	for _, scope := range aprsScopes {
		var count int64
		scope.scopeWhere(s.db.Model(&dbAPRSSent{}), "to").Count(&count)
		sum += count
	}
	if sum != total {
		t.Fatalf("sections hold %d of %d sent messages", sum, total)
	}
}

// A page shows only its own section, and its total matches the section.
func TestSectionPagesShowOnlyTheirOwnMessages(t *testing.T) {
	s := newTestServer(t)
	user := seedScopedAPRS(t, s)

	direct := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received", nil), user)
	if !strings.Contains(direct.Body.String(), "from a person") {
		t.Fatal("the direct section does not show a message from another station")
	}
	for _, bot := range []string{"N:APRSPH 3 Messages Sent", "N:HOTG 239 Messages Sent"} {
		if strings.Contains(direct.Body.String(), bot) {
			t.Fatalf("the direct section shows bot traffic: %q", bot)
		}
	}

	aprsph := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=aprsph", nil), user)
	if !strings.Contains(aprsph.Body.String(), "N:APRSPH 3 Messages Sent") {
		t.Fatal("the APRSPH section does not show the bot's message")
	}
	if strings.Contains(aprsph.Body.String(), "from a person") {
		t.Fatal("the APRSPH section shows a message from another station")
	}

	thursday := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=thursday", nil), user)
	for _, want := range []string{"N:HOTG 239 Messages Sent", "ack1B}"} {
		if !strings.Contains(thursday.Body.String(), want) {
			t.Fatalf("the Thursday section does not show %q — an ack names no group, so it must still land here", want)
		}
	}
}

// An unknown section is a 404 rather than a silent fallback, so a stale link says
// what is wrong instead of showing the wrong list.
func TestUnknownSectionIsNotFound(t *testing.T) {
	s := newTestServer(t)
	user := seedScopedAPRS(t, s)
	response := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=nonsense", nil), user)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown section = %d, want 404", response.Code)
	}
}

// The tab bar counts each section and marks the current one.
func TestSectionTabs(t *testing.T) {
	s := newTestServer(t)
	user := seedScopedAPRS(t, s)
	s.text["en"] = map[string]any{
		"aprs_section_direct":        "Other stations",
		"aprs_section_aprsph":        "APRSPH net",
		"aprs_section_thursday":      "#APRSThursday",
		"aprs_section_direct_desc":   "Messages exchanged with other stations.",
		"aprs_section_aprsph_desc":   "Traffic with the APRSPH net bot.",
		"aprs_section_thursday_desc": "Group-server traffic.",
	}
	s.tpl = parseTemplates(s.text)

	body := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=aprsph", nil), user).Body.String()
	if !strings.Contains(body, `href="/aprs/received?section=aprsph" aria-current="page"`) {
		t.Fatal("the current section is not marked")
	}
	if !strings.Contains(body, `href="/aprs/received"`) {
		t.Fatal("the direct section has no link, or it is not the plain URL")
	}
	// The counts must be the section counts, not the whole history.
	if !strings.Contains(body, "<b>2</b>") || !strings.Contains(body, "<b>1</b>") {
		t.Fatalf("the tab counts do not look like section counts: %s", body)
	}
}

// Pagination must stay inside the section: a link that drops the parameter would
// silently jump to the direct list.
func TestSectionPaginationKeepsTheSection(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "op@example.invalid", EnableAPRS: true})
	for i := 0; i < 25; i++ {
		if err := s.db.Create(&dbAPRSReceived{
			UserCallsign: "EA7KLK", Position: i, At: now(), From: "APRSPH", To: "EA7KLK", Text: "bot message",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	body := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=aprsph&per_page=10", nil), user).Body.String()
	if !strings.Contains(body, "?section=aprsph&amp;page=2") && !strings.Contains(body, "?section=aprsph&page=2") {
		t.Fatalf("a next-page link drops the section: %s", body)
	}
	// The page-size form is a GET with no action, so it needs the section as a
	// hidden field or the choice is lost on submit.
	if !strings.Contains(body, `name="section" value="aprsph"`) {
		t.Fatal("the page-size form does not carry the section")
	}
}

// The scope is derived from the stored peer, so a new section needs no migration
// and no stored column: a row written by the terminal lands in the right place.
func TestSectionAppliesToRowsWrittenByTheTerminal(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "op@example.invalid", EnableAPRS: true})
	// Exactly the shape the SSH receiver writes.
	if err := s.db.Create(&dbAPRSReceived{
		UserCallsign: "EA7KLK", Position: 0, At: now(),
		From: "APRSPH", To: "EA7KLK-0", Text: "N:APRSPH 12 Messages Sent", Raw: "APRSPH>APRS::EA7KLK-0:N:APRSPH 12 Messages Sent{AB12",
	}).Error; err != nil {
		t.Fatal(err)
	}
	body := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=aprsph", nil), user).Body.String()
	if !strings.Contains(body, "N:APRSPH 12 Messages Sent") {
		t.Fatal("a row written by the terminal did not appear in the APRSPH section")
	}
}

// A link from one direction's section opens the same section in the other.
func TestCrossDirectionLinkKeepsTheSection(t *testing.T) {
	s := newTestServer(t)
	user := seedScopedAPRS(t, s)
	body := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=thursday", nil), user).Body.String()
	link := "href=\"/aprs/sent?section=thursday\""
	if !strings.Contains(body, link) && !strings.Contains(body, "href=\"/aprs/sent?section=thursday&amp;") {
		t.Fatalf("the sent link does not keep the section: %s", body)
	}
}

func TestAPRSSectionPaths(t *testing.T) {
	if got := aprsListPath("sent", aprsScopeDirect); got != "/aprs/sent" {
		t.Fatalf("direct path = %q", got)
	}
	if got := aprsListPath("sent", aprsScopeAPRSPH); got != "/aprs/sent?section=aprsph" {
		t.Fatalf("aprsph path = %q", got)
	}
	// Pagination appends with the right separator whichever form the base takes.
	for _, tc := range []struct{ path, want string }{
		{"/aprs/sent", "/aprs/sent?page=2&per_page=10"},
		{"/aprs/sent?section=aprsph", "/aprs/sent?section=aprsph&page=2&per_page=10"},
	} {
		if got := aprsPageURL(tc.path, 2, 10); got != tc.want {
			t.Fatalf("aprsPageURL(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}
