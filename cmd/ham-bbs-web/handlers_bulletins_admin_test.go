package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBulletinBusinessLogic(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid", IsSysop: true})
	if got := invokeUserHandler(s.bulletins, formRequest(http.MethodGet, "/bulletins", nil), user); got.Code != http.StatusOK {
		t.Fatalf("bulletins status = %d", got.Code)
	}
	if got := invokeUserHandler(s.bulletinNew, formRequest(http.MethodGet, "/bulletins/new", nil), user); got.Code != http.StatusOK {
		t.Fatalf("bulletinNew status = %d", got.Code)
	}
	invalid := invokeUserHandler(s.bulletinCreate, formRequest(http.MethodPost, "/bulletins/new", url.Values{"title": {"Title"}}), user)
	if invalid.Code != http.StatusOK || !strings.Contains(invalid.Body.String(), "required") {
		t.Fatalf("invalid bulletin response = %d %q", invalid.Code, invalid.Body.String())
	}
	created := invokeUserHandler(s.bulletinCreate, formRequest(http.MethodPost, "/bulletins/new", url.Values{"title": {" Title "}, "body": {" Body "}}), user)
	assertRedirect(t, created, "/bulletins")
	var item dbBulletin
	if err := s.db.First(&item).Error; err != nil {
		t.Fatal(err)
	}
	if item.Position != 0 || item.Title != "Title" || item.Body != "Body" || item.From != user.Callsign || item.Updated == "" {
		t.Fatalf("created bulletin = %#v", item)
	}
	if got := invokeUserHandler(s.bulletinEdit, setPathValue(formRequest(http.MethodGet, "/bulletins/1/edit", nil), "id", "1"), user); got.Code != http.StatusOK {
		t.Fatalf("bulletinEdit status = %d", got.Code)
	}
	if got := invokeUserHandler(s.bulletinEdit, setPathValue(formRequest(http.MethodGet, "/bulletins/missing/edit", nil), "id", "bad"), user); got.Code != http.StatusNotFound {
		t.Fatalf("missing bulletinEdit status = %d", got.Code)
	}
	invalidUpdate := invokeUserHandler(s.bulletinUpdate, setPathValue(formRequest(http.MethodPost, "/bulletins/1/edit", url.Values{"title": {""}, "body": {"Body"}}), "id", "1"), user)
	if invalidUpdate.Code != http.StatusOK || !strings.Contains(invalidUpdate.Body.String(), "required") {
		t.Fatalf("invalid bulletin update = %d %q", invalidUpdate.Code, invalidUpdate.Body.String())
	}
	updated := invokeUserHandler(s.bulletinUpdate, setPathValue(formRequest(http.MethodPost, "/bulletins/1/edit", url.Values{"title": {" New title "}, "body": {" New body "}}), "id", "1"), user)
	assertRedirect(t, updated, "/bulletins")
	if err := s.db.First(&item, 1).Error; err != nil {
		t.Fatal(err)
	}
	if item.Title != "New title" || item.Body != "New body" || item.From != user.Callsign {
		t.Fatalf("updated bulletin = %#v", item)
	}
	missingUpdate := invokeUserHandler(s.bulletinUpdate, setPathValue(formRequest(http.MethodPost, "/bulletins/bad/edit", url.Values{"title": {"x"}, "body": {"y"}}), "id", "bad"), user)
	if missingUpdate.Code != http.StatusNotFound {
		t.Fatalf("missing bulletinUpdate status = %d", missingUpdate.Code)
	}
	deleted := invokeUserHandler(s.bulletinDelete, setPathValue(formRequest(http.MethodPost, "/bulletins/1/delete", nil), "id", "1"), user)
	assertRedirect(t, deleted, "/bulletins")
	if _, err := s.findBulletin(setPathValue(formRequest(http.MethodGet, "/bulletins/1", nil), "id", "1")); err == nil {
		t.Fatal("deleted bulletin still exists")
	}
	missingDelete := invokeUserHandler(s.bulletinDelete, setPathValue(formRequest(http.MethodPost, "/bulletins/bad/delete", nil), "id", "bad"), user)
	assertRedirect(t, missingDelete, "/bulletins")
}

func TestAdminUserBusinessLogic(t *testing.T) {
	s := newTestServer(t)
	s.cfg.sysops["EA7KLK"] = true
	admin := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Admin", Email: "admin@example.invalid", IsSysop: true})
	target := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Target", Email: "target@example.invalid"})
	if got := invokeUserHandler(s.adminUsers, formRequest(http.MethodGet, "/admin/users", nil), admin); got.Code != http.StatusOK {
		t.Fatalf("adminUsers status = %d", got.Code)
	}
	if s.wouldRemoveLastSysop(target) {
		t.Fatal("ordinary user cannot remove the last sysop")
	}
	target.IsSysop = true
	if s.wouldRemoveLastSysop(target) {
		t.Fatal("another active sysop should prevent last-sysop detection")
	}
	target.Disabled = true
	if s.wouldRemoveLastSysop(target) {
		t.Fatal("disabled target should not trigger last-sysop protection")
	}
	onlySysopServer := newTestServer(t)
	onlySysop := testUser(t, onlySysopServer, dbUser{Callsign: "EA2XYZ", FullName: "Only Sysop", Email: "sysop@example.invalid", IsSysop: true})
	if !onlySysopServer.wouldRemoveLastSysop(onlySysop) {
		t.Fatal("only active sysop was not protected")
	}

	self := invokeUserHandler(s.adminUserUpdate, setPathValue(formRequest(http.MethodPost, "/admin/users/EA7KLK", url.Values{"disabled": {"true"}}), "callsign", "EA7KLK"), admin)
	assertRedirect(t, self, "/admin/users")
	var savedAdmin dbUser
	if err := s.db.First(&savedAdmin, "callsign = ?", admin.Callsign).Error; err != nil {
		t.Fatal(err)
	}
	if savedAdmin.Disabled {
		t.Fatal("admin was allowed to disable itself")
	}

	ordinary := invokeUserHandler(s.adminUserUpdate, setPathValue(formRequest(http.MethodPost, "/admin/users/EA1ABC", url.Values{"is_sysop": {"true"}, "disabled": {"true"}}), "callsign", "EA1ABC"), admin)
	assertRedirect(t, ordinary, "/admin/users")
	var savedTarget dbUser
	if err := s.db.First(&savedTarget, "callsign = ?", target.Callsign).Error; err != nil {
		t.Fatal(err)
	}
	if !savedTarget.IsSysop || !savedTarget.Disabled {
		t.Fatalf("ordinary admin update = %#v", savedTarget)
	}

	protected := invokeUserHandler(s.adminUserUpdate, setPathValue(formRequest(http.MethodPost, "/admin/users/EA1ABC", url.Values{"disabled": {"true"}}), "callsign", "EA1ABC"), admin)
	assertRedirect(t, protected, "/admin/users")
	unknown := invokeUserHandler(s.adminUserUpdate, setPathValue(formRequest(http.MethodPost, "/admin/users/UNKNOWN", url.Values{"disabled": {"true"}}), "callsign", "UNKNOWN"), admin)
	assertRedirect(t, unknown, "/admin/users")
}
