package main

import (
	"net/http"
	"net/url"
	"testing"
)

func registerWithForm(t *testing.T, s *server, form url.Values) *dbUser {
	t.Helper()
	invokeHandler(s.registerPost, formRequest(http.MethodPost, "/register", form))
	var saved dbUser
	if err := s.db.First(&saved, "callsign = ?", normalizeCallsign(form.Get("callsign"))).Error; err != nil {
		t.Fatalf("registration of %q did not store an account: %v", form.Get("callsign"), err)
	}
	return &saved
}

func TestConfiguredSysopIsABootstrapGrantOnly(t *testing.T) {
	s := newTestServer(t)
	s.cfg.sysops["EA7KLK"] = true
	saved := registerWithForm(t, s, validUserForm())
	if !saved.IsSysop {
		t.Fatal("the first account in BBS_SYSOPS was not granted sysop on an empty BBS")
	}
}

func TestConfiguredSysopIsIgnoredOnceASysopExists(t *testing.T) {
	s := newTestServer(t)
	s.cfg.sysops["EA7KLK"] = true
	testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Existing", Email: "a@example.invalid", IsSysop: true})

	saved := registerWithForm(t, s, validUserForm())
	if saved.IsSysop {
		t.Fatal("registration handed sysop to a configured callsign while a sysop existed")
	}
}

func TestDisabledSysopDoesNotCloseTheBootstrapWindow(t *testing.T) {
	s := newTestServer(t)
	s.cfg.sysops["EA7KLK"] = true
	testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Disabled", Email: "a@example.invalid", IsSysop: true, Disabled: true})

	if !s.isBootstrapSysop("EA7KLK") {
		t.Fatal("a disabled sysop must not count as an active sysop")
	}
}

func TestApplyConfiguredSysopsPromotesOnlyWithoutAnActiveSysop(t *testing.T) {
	s := newTestServer(t)
	s.cfg.sysops["EA7KLK"] = true
	testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Configured", Email: "k@example.invalid"})

	promoted := s.applyConfiguredSysops()
	if len(promoted) != 1 || promoted[0] != "EA7KLK" {
		t.Fatalf("applyConfiguredSysops() = %#v", promoted)
	}
	if !userFromDBIsSysop(t, s, "EA7KLK") {
		t.Fatal("bootstrap sysop was not stored")
	}

	// With an active sysop present the configured list has no authority.
	other := newTestServer(t)
	other.cfg.sysops["EA7KLK"] = true
	testUser(t, other, dbUser{Callsign: "EA1ABC", FullName: "Existing", Email: "a@example.invalid", IsSysop: true})
	testUser(t, other, dbUser{Callsign: "EA7KLK", FullName: "Configured", Email: "k@example.invalid"})
	if promoted := other.applyConfiguredSysops(); len(promoted) != 0 {
		t.Fatalf("applyConfiguredSysops() promoted %#v while a sysop existed", promoted)
	}
	if userFromDBIsSysop(t, other, "EA7KLK") {
		t.Fatal("configured callsign was promoted while a sysop existed")
	}
}

// The last sysop cannot lock the BBS out through this endpoint: a sysop's own
// row is never changed, and every caller of the handler is itself an active
// sysop, so at least one active sysop always remains.
func TestSysopCannotChangeTheirOwnRole(t *testing.T) {
	s := newTestServer(t)
	actor := testUser(t, s, dbUser{Callsign: "EA1AAA", FullName: "Actor", Email: "a@example.invalid", IsSysop: true})

	response := invokeUserHandler(s.adminUserUpdate, setPathValue(formRequest(http.MethodPost, "/admin/users/EA1AAA", url.Values{"is_sysop": {"false"}, "disabled": {"true"}}), "callsign", "EA1AAA"), actor)
	assertRedirect(t, response, "/admin/users")
	if !userFromDBIsSysop(t, s, actor.Callsign) {
		t.Fatal("a sysop demoted their own account")
	}
	var stored dbUser
	if err := s.db.First(&stored, "callsign = ?", actor.Callsign).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Disabled {
		t.Fatal("a sysop disabled their own account")
	}
}

func TestConfiguredSysopCanBeDemoted(t *testing.T) {
	s := newTestServer(t)
	s.cfg.sysops["EA7KLK"] = true
	actor := testUser(t, s, dbUser{Callsign: "EA1AAA", FullName: "Actor", Email: "a@example.invalid", IsSysop: true})
	testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Configured", Email: "k@example.invalid", IsSysop: true})

	form := url.Values{"is_sysop": {"false"}, "disabled": {"false"}}
	response := invokeUserHandler(s.adminUserUpdate, setPathValue(formRequest(http.MethodPost, "/admin/users/EA7KLK", form), "callsign", "EA7KLK"), actor)
	assertRedirect(t, response, "/admin/users")
	if userFromDBIsSysop(t, s, "EA7KLK") {
		t.Fatal("a configured sysop could not be demoted")
	}
}
