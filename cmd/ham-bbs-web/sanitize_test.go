package main

import "testing"

func TestSanitizeTerminalTextRemovesControlAndSpoofingCharacters(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		single string
		multi  string
	}{
		{"csi clear screen", "hi\x1b[2Jthere", "hi[2Jthere", "hi[2Jthere"},
		{"osc 52 clipboard", "a\x1b]52;c;ZXZpbA==\x07b", "a]52;c;ZXZpbA==b", "a]52;c;ZXZpbA==b"},
		{"bare escape", "a\x1bb", "ab", "ab"},
		{"c1 csi", "a\u009b2Jb", "a2Jb", "a2Jb"},
		{"c1 control", "before \u009f after", "before after", "before  after"},
		{"del", "a\x7fb", "ab", "ab"},
		{"right-to-left override", "abc\u202edef", "abcdef", "abcdef"},
		{"isolate", "abc\u2067def\u2069", "abcdef", "abcdef"},
		{"zero width space", "a\u200bb", "ab", "ab"},
		{"byte order mark", "\ufeffa", "a", "a"},
		{"tab", "a\tb", "a b", "a b"},
		{"newline in single line", "a\nb", "a b", "a\nb"},
		{"crlf normalised", "a\r\nb", "a b", "a\nb"},
		{"lone cr normalised", "a\rb", "a b", "a\nb"},
		{"blank line run capped", "a\n\n\n\nb", "a b", "a\n\nb"},
		{"invalid utf8", "a\xffb", "a\uFFFDb", "a\uFFFDb"},
		{"accents kept", "Müller", "Müller", "Müller"},
		{"cjk kept", "日本語", "日本語", "日本語"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeSingleLineText(tc.in); got != tc.single {
				t.Fatalf("sanitizeSingleLineText(%q) = %q, want %q", tc.in, got, tc.single)
			}
			if got := sanitizeMultilineText(tc.in); got != tc.multi {
				t.Fatalf("sanitizeMultilineText(%q) = %q, want %q", tc.in, got, tc.multi)
			}
		})
	}
}

func TestSanitizeTerminalTextIsIdempotent(t *testing.T) {
	inputs := []string{"hi\x1b[2Jthere", "a\n\n\n\nb", "x\x1b]52;c;eA==\x07y", "abc\u202edef", "a\xffb"}
	for _, in := range inputs {
		once := sanitizeMultilineText(in)
		if twice := sanitizeMultilineText(once); twice != once {
			t.Fatalf("multiline not idempotent for %q: %q then %q", in, once, twice)
		}
		if twice := sanitizeSingleLineText(sanitizeSingleLineText(in)); twice != sanitizeSingleLineText(in) {
			t.Fatalf("single line not idempotent for %q", in)
		}
	}
}

// Rows written before this rule existed, or by a client that bypassed it, are
// cleaned on read so the browser never renders them.
func TestLegacyStoredTextIsCleanedOnLoad(t *testing.T) {
	s := newTestServer(t)
	payload := "hi\x1b[2Jthere"
	if err := s.db.Create(&dbBoard{ID: "general", Position: 0, Name: "Gen\x1b[2Jeral", Description: "d\x1b[2J"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&dbMessage{BoardID: "general", From: "EA7KLK", Subject: payload, Body: payload + "\n\n\n\nx", Created: now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&dbBulletin{Position: 0, Title: payload, Body: payload, Updated: now()}).Error; err != nil {
		t.Fatal(err)
	}

	boards := s.loadBoards()
	if len(boards) != 1 {
		t.Fatalf("loadBoards() = %#v", boards)
	}
	if boards[0].Name != "Gen[2Jeral" {
		t.Fatalf("loaded board name = %q", boards[0].Name)
	}
	nodes := s.messageTree("general")
	if len(nodes) != 1 {
		t.Fatalf("messageTree() = %#v", nodes)
	}
	if nodes[0].Row.Subject != "hi[2Jthere" || nodes[0].Row.Body != "hi[2Jthere\n\nx" {
		t.Fatalf("loaded message = %#v", nodes[0].Row)
	}
	bulletins := s.loadBulletins()
	if len(bulletins) != 1 || bulletins[0].Title != "hi[2Jthere" {
		t.Fatalf("loadBulletins() = %#v", bulletins)
	}
}
