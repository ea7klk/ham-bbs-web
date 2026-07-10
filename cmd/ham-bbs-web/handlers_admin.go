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
		if !s.cfg.sysops[target.Callsign] {
			target.IsSysop = r.FormValue("is_sysop") == "true"
		}
		target.Disabled = r.FormValue("disabled") == "true"
		if target.Disabled && s.wouldRemoveLastSysop(&target) {
			target.Disabled = false
		}
		s.db.Save(&target)
		s.logBBSAction(user.Callsign, "web_user_update", "target=%q disabled=%t sysop=%t", target.Callsign, target.Disabled, target.IsSysop)
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (s *server) wouldRemoveLastSysop(target *dbUser) bool {
	if !s.isSysop(target) || target.Disabled {
		return false
	}
	var users []dbUser
	s.db.Find(&users)
	count := 0
	for i := range users {
		if users[i].Callsign == target.Callsign {
			continue
		}
		if s.isSysop(&users[i]) && !users[i].Disabled {
			count++
		}
	}
	return count == 0
}
