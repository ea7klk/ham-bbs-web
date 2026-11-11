package main

import (
	"strings"
	"testing"
)

// The placeholder rule is a decision about whether the deployment is finished,
// not a formatting detail: it decides whether registration requires an emailed
// code. It has to give the same answer as the classic SSH BBS, or the two
// surfaces disagree about whether an account is verified.
func TestPlaceholderValuesAreRefused(t *testing.T) {
	placeholders := []string{
		"", "   ",
		"changeme", "change-me", "change_me", "password", "placeholder", "secret", "todo", "xxx",
		"change-this-smtp-password", "change_this_password", "Change-This-Password",
		"__FILL_ME__", "<your-password>", "{{ password }}",
		"replace_with_password", "replace-with-me", "set_me", "setme", "insert_password",
	}
	for _, value := range placeholders {
		if !isPlaceholderValue(value) {
			t.Fatalf("isPlaceholderValue(%q) = false, want true", value)
		}
	}
}

// ...but a real password is not a placeholder just because it contains an
// ordinary word. This is why the bare words are matched exactly.
func TestRealPasswordsAreAccepted(t *testing.T) {
	real := []string{
		"hunter2-and-then-some",
		"correct horse battery staple",
		"mysecretvalue2026",           // contains "secret" but is not the word
		"Tr0ub4dor&3",                 // conventional strength example
		"placeholder_production_only", // contains "placeholder" but is not it
		"x9f2Kq!mZ",
	}
	for _, value := range real {
		if isPlaceholderValue(value) {
			t.Fatalf("isPlaceholderValue(%q) = true, a real password was refused", value)
		}
	}
}

func TestMailEnabledRequiresACompleteConfiguration(t *testing.T) {
	complete := func() mailConfig {
		return mailConfig{
			host:       "mail.example.invalid",
			port:       587,
			username:   "bbs@example.invalid",
			password:   "a-real-password",
			from:       "bbs@example.invalid",
			fromName:   "Test BBS",
			security:   "starttls",
			webBaseURL: "https://bbs.example.invalid",
		}
	}

	if problem := (&mailer{cfg: complete()}).Problem(); problem != "" {
		t.Fatalf("a complete configuration reported %q", problem)
	}

	cases := map[string]func(*mailConfig){
		"no host":               func(c *mailConfig) { c.host = "" },
		"no sender":             func(c *mailConfig) { c.from = "" },
		"sender without @":      func(c *mailConfig) { c.from = "bbs" },
		"bad port":              func(c *mailConfig) { c.port = 0 },
		"bad security":          func(c *mailConfig) { c.security = "ssl" },
		"placeholder password":  func(c *mailConfig) { c.password = "__FILL_ME__" },
		"relative web base URL": func(c *mailConfig) { c.webBaseURL = "bbs.example.invalid" },
	}
	for name, mutate := range cases {
		cfg := complete()
		mutate(&cfg)
		if problem := (&mailer{cfg: cfg}).Problem(); problem == "" {
			t.Fatalf("%s: Problem() = \"\", want a reason", name)
		}
	}

	// A relay that needs no authentication stays valid: only a username makes
	// the password load-bearing.
	relay := complete()
	relay.username = ""
	relay.password = ""
	if problem := (&mailer{cfg: relay}).Problem(); problem != "" {
		t.Fatalf("an unauthenticated relay reported %q", problem)
	}
}

// Without a usable mailer a send fails loudly rather than silently doing
// nothing, so a caller cannot report success on a lost code.
func TestSendFailsWhenMailIsDisabled(t *testing.T) {
	m := &mailer{cfg: mailConfig{}}
	if m.Enabled() {
		t.Fatal("an empty configuration reported itself enabled")
	}
	if err := m.Send(mailMessage{To: "someone@example.invalid"}); err == nil {
		t.Fatal("Send() reported success with mail disabled")
	}
}

func TestMailLinksUseTheWebBaseURL(t *testing.T) {
	m := &mailer{cfg: mailConfig{webBaseURL: "https://bbs.example.invalid/"}}
	if got := m.WebURL("/reset?token=abc"); got != "https://bbs.example.invalid/reset?token=abc" {
		t.Fatalf("WebURL() = %q", got)
	}
	if got := m.WebURL("reset?token=abc"); got != "https://bbs.example.invalid/reset?token=abc" {
		t.Fatalf("WebURL() without a leading slash = %q", got)
	}
}

// The four mail bodies come from the classic BBS; check each one carries the
// values it is given rather than a stray formatting verb.
func TestMailBodiesCarryTheirValues(t *testing.T) {
	activation := activationMail("en", "EA1ABC", "S5ZD", "Test BBS")
	if activation.Subject == "" || !strings.Contains(activation.Body, "S5ZD") || !strings.Contains(activation.Body, "EA1ABC") {
		t.Fatalf("activation mail = %#v", activation)
	}
	reset := passwordResetMail("de", "EA1ABC", "https://bbs.example.invalid/reset?token=x", "Test BBS")
	if !strings.Contains(reset.Body, "https://bbs.example.invalid/reset?token=x") {
		t.Fatalf("reset mail lost the link: %q", reset.Body)
	}
	old := emailChangeMail("en", "EA1ABC", "https://x/1", "Test BBS", "new@example.invalid", true)
	if !strings.Contains(old.Body, "new@example.invalid") {
		t.Fatalf("approval mail does not name the new address: %q", old.Body)
	}
	// A link longer than the wrap width must not be broken, or it stops being
	// clickable in a plain-text client.
	if !strings.Contains(reset.Body, "https://bbs.example.invalid/reset?token=x") {
		t.Fatal("the reset link was wrapped and would not be clickable")
	}
}
