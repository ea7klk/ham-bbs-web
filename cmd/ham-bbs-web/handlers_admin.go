package main

import (
	"net/http"
)

func (s *server) adminUsers(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var users []dbUser
	s.db.Order("callsign").Find(&users)
	s.view(w, r, "admin_users", user, users, "", "")
}

func (s *server) adminUserUpdate(w http.ResponseWriter, r *http.Request, user *dbUser) {
	callsign := normalizeCallsign(r.PathValue("callsign"))
	if callsign == user.Callsign {
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}
	var target dbUser
	if err := s.db.First(&target, "callsign = ?", callsign).Error; err == nil {
		// A bootstrapped sysop is an ordinary sysop, so any sysop may be
		// demoted; the last-sysop guard below still prevents locking the BBS out.
		target.IsSysop = r.FormValue("is_sysop") == "true"
		target.Disabled = r.FormValue("disabled") == "true"
		// Name the two columns this form owns. A whole-row Save would rewrite the
		// write-once registration date from a value read a moment earlier, and
		// would clobber any column another session changed in between.
		s.db.Model(&dbUser{}).Where("callsign = ?", target.Callsign).
			Updates(map[string]any{"is_sysop": target.IsSysop, "disabled": target.Disabled})
		s.logBBSAction(user.Callsign, "web_user_update", "target=%q disabled=%t sysop=%t", target.Callsign, target.Disabled, target.IsSysop)
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}
