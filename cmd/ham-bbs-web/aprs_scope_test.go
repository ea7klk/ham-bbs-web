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

// The classifier is content-first, peer-second. Every case is a shape observed in
// the live database; the first version of this rule missed the majority of the
// Thursday traffic because it classified by peer alone.
func TestAPRSScopeOfMessage(t *testing.T) {
	cases := []struct {
		name string
		peer string
		text string
		want aprsScope
	}{
		// The actual #APRSThursday conversation: relayed by participants.
		{"relayed by a participant", "KB3KBR-2", "N:HOTG happy aprsthursday!!!", aprsScopeThursday},
		{"relayed, another station", "HI3RRR-7", "N:HOTG Saludos desde la Republica Dominicana", aprsScopeThursday},
		{"relayed from the net bot", "APRSPH", "N:HOTG net message", aprsScopeThursday},

		// The group server itself, for Thursday.
		{"ansrvr summary", "ANSRVR", "N:HOTG 24 Messages Sent", aprsScopeThursday},
		{"ansrvr removal", "ANSRVR", "N:HOTG Removed From Group{GJ}", aprsScopeThursday},

		// Other groups on the same server are not Thursday.
		{"other group", "ANSRVR", "N:AARN Check in", aprsScopeGroups},
		{"other group removed", "ANSRVR", "N:NEAPRS Removed From Group{UT}", aprsScopeGroups},
		{"other group relayed", "TI2BSH-15", "N:NEAPRS hello", aprsScopeGroups},

		// An acknowledgement names no group.
		{"acknowledgement", "ANSRVR", "ack1B}", aprsScopeGroups},

		// Our own outgoing commands.
		{"join command", "ANSRVR", "CQ HOTG", aprsScopeThursday},
		{"join with text", "ANSRVR", "CQ HOTG hello from Madrid", aprsScopeThursday},
		{"keepalive", "ANSRVR", "K HOTG", aprsScopeThursday},
		{"leave", "ANSRVR", "U HOTG", aprsScopeThursday},

		// The APRSPH bot, by peer.
		{"aprsph reply", "APRSPH", "1113 0501Z:QSP 8 stations. LIST for recipients.", aprsScopeAPRSPH},
		{"aprsph checkin ack", "APRSPH", "Ur checked in 1113 0204Z. QRX pls til 2359Z.", aprsScopeAPRSPH},
		{"aprsph with ssid", "APRSPH-0", "hello", aprsScopeAPRSPH},

		// Direct traffic, including text that merely looks like a tag.
		{"person", "DN9RZ", "hello", aprsScopeDirect},
		{"person with ssid", "DN9RZ-10", "hi", aprsScopeDirect},
		{"N: but no group", "DN9RZ", "N: hello", aprsScopeDirect},
		{"N: with too long a tag", "DN9RZ", "N:ABCDEFGHI text", aprsScopeDirect},
		{"hotg in ordinary prose", "DN9RZ", "join us on hotg tomorrow", aprsScopeDirect},
		{"callsign that starts the same", "APRSPHX", "hello", aprsScopeDirect},
	}
	for _, tc := range cases {
		if got := aprsScopeOfMessage(tc.peer, tc.text); got != tc.want {
			t.Fatalf("%s: aprsScopeOfMessage(%q, %q) = %q, want %q",
				tc.name, tc.peer, tc.text, got, tc.want)
		}
	}
}

func TestAPRSGroupTag(t *testing.T) {
	cases := map[string]struct {
		group string
		ok    bool
	}{
		"N:HOTG 24 Messages Sent": {"HOTG", true},
		"N:HOTG":                  {"HOTG", true},
		"N:AARN hello":            {"AARN", true},
		"N:HOTG{a}":               {"HOTG", true},
		"n:hotg lower":            {"HOTG", true},
		"N:NEAPRS hello":          {"NEAPRS", true},
		"N:ABCDEFGH text":         {"ABCDEFGH", true},
		"N:":                      {"", false},
		"N:ABCDEFGHI text":        {"", false},
		"ack1B}":                  {"", false},
		"hello":                   {"", false},
	}
	for text, want := range cases {
		group, ok := aprsGroupTag(text)
		if ok != want.ok || group != want.group {
			t.Fatalf("aprsGroupTag(%q) = %q,%t; want %q,%t", text, group, ok, want.group, want.ok)
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
		{UserCallsign: "EA7KLK", Position: 2, At: now(), From: "APRSPH", To: "EA7KLK", Text: "1113 0501Z:QSP 8 stations"},
		{UserCallsign: "EA7KLK", Position: 3, At: now(), From: "ANSRVR", To: "EA7KLK", Text: "N:HOTG 239 Messages Sent"},
		{UserCallsign: "EA7KLK", Position: 4, At: now(), From: "ANSRVR", To: "EA7KLK", Text: "ack1B}"},
		// Relayed by a participant, which is how the Thursday net actually
		// arrives — the case the peer-only rule put in "other stations".
		{UserCallsign: "EA7KLK", Position: 5, At: now(), From: "KB3KBR-2", To: "EA7KLK", Text: "N:HOTG happy aprsthursday!!!"},
		{UserCallsign: "EA7KLK", Position: 6, At: now(), From: "ANSRVR", To: "EA7KLK", Text: "N:AARN Check in"},
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

// The classification is one function used by the list, the counts and the tabs,
// so the three cannot disagree. This checks that it partitions a realistic
// history exactly: every message in one section, none in two, none missing.
func TestScopePartitionsTheHistory(t *testing.T) {
	s := newTestServer(t)
	seedScopedAPRS(t, s)

	received := s.loadReceivedHistory("EA7KLK")
	sum := int64(0)
	for _, scope := range aprsScopes {
		count := int64(0)
		for _, row := range received {
			if aprsScopeOfMessage(row.From, row.Text) == scope {
				count++
			}
		}
		sum += count
	}
	if sum != int64(len(received)) {
		t.Fatalf("sections hold %d of %d received messages", sum, len(received))
	}

	sent := s.loadSentHistory("EA7KLK")
	sum = 0
	for _, scope := range aprsScopes {
		count := int64(0)
		for _, row := range sent {
			if aprsScopeOfMessage(row.To, row.Text) == scope {
				count++
			}
		}
		sum += count
	}
	if sum != int64(len(sent)) {
		t.Fatalf("sections hold %d of %d sent messages", sum, len(sent))
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
	for _, bot := range []string{"1113 0501Z:QSP 8 stations", "N:HOTG 239 Messages Sent"} {
		if strings.Contains(direct.Body.String(), bot) {
			t.Fatalf("the direct section shows bot traffic: %q", bot)
		}
	}

	aprsph := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=aprsph", nil), user)
	if !strings.Contains(aprsph.Body.String(), "1113 0501Z:QSP 8 stations") {
		t.Fatal("the APRSPH section does not show the bot's message")
	}
	if strings.Contains(aprsph.Body.String(), "from a person") {
		t.Fatal("the APRSPH section shows a message from another station")
	}

	thursday := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=thursday", nil), user)
	if !strings.Contains(thursday.Body.String(), "N:HOTG 239 Messages Sent") {
		t.Fatal("the Thursday section does not show the tagged group message")
	}
	// The relayed message is the point: it came from another station, and the
	// peer-only rule would have left it in "other stations".
	if !strings.Contains(thursday.Body.String(), "N:HOTG happy aprsthursday!!!") {
		t.Fatal("the Thursday section does not show a message relayed by a participant")
	}
	if strings.Contains(thursday.Body.String(), "N:AARN Check in") {
		t.Fatal("another group's traffic was filed under Thursday")
	}
	// An acknowledgement names no group. It must not be guessed into Thursday;
	// it belongs with the rest of the group server's ungrouped traffic.
	if strings.Contains(thursday.Body.String(), "ack1B}") {
		t.Fatal("an acknowledgement was filed under Thursday")
	}
	groups := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=groups", nil), user)
	if !strings.Contains(groups.Body.String(), "ack1B}") {
		t.Fatal("the acknowledgement did not land with the group server")
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
	// "N:APRSPH" is a group tag, so this row is group traffic, not bot traffic —
	// which is exactly the distinction that matters and that the first version of
	// the rule got wrong in the other direction.
	groups := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received?section=groups", nil), user).Body.String()
	if !strings.Contains(groups, "N:APRSPH 12 Messages Sent") {
		t.Fatal("a tagged row written by the terminal did not appear in the groups section")
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
