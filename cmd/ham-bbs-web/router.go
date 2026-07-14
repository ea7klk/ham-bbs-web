package main

import (
	"net/http"
)

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.requireAuth(s.home))
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("GET /register", s.registerForm)
	mux.HandleFunc("POST /register", s.registerPost)
	mux.HandleFunc("POST /logout", s.logoutPost)
	mux.HandleFunc("POST /language", s.requireAuth(s.languagePost))
	mux.HandleFunc("GET /profile", s.requireAuth(s.profileForm))
	mux.HandleFunc("POST /profile", s.requireAuth(s.profilePost))
	mux.HandleFunc("GET /directory", s.requireAuth(s.directory))
	mux.HandleFunc("GET /bulletins", s.requireAuth(s.bulletins))
	mux.HandleFunc("GET /bulletins/new", s.requireSysop(s.bulletinNew))
	mux.HandleFunc("POST /bulletins/new", s.requireSysop(s.bulletinCreate))
	mux.HandleFunc("GET /bulletins/{id}/edit", s.requireSysop(s.bulletinEdit))
	mux.HandleFunc("POST /bulletins/{id}/edit", s.requireSysop(s.bulletinUpdate))
	mux.HandleFunc("POST /bulletins/{id}/delete", s.requireSysop(s.bulletinDelete))
	mux.HandleFunc("GET /boards", s.requireAuth(s.boards))
	mux.HandleFunc("GET /boards/new", s.requireSysop(s.boardNew))
	mux.HandleFunc("POST /boards/new", s.requireSysop(s.boardCreate))
	mux.HandleFunc("GET /boards/{id}", s.requireAuth(s.boardShow))
	mux.HandleFunc("POST /boards/{id}/post", s.requireAuth(s.messagePost))
	mux.HandleFunc("POST /messages/{id}/reply", s.requireAuth(s.messageReply))
	mux.HandleFunc("POST /messages/{id}/delete", s.requireSysop(s.messageDelete))
	mux.HandleFunc("GET /boards/{id}/edit", s.requireSysop(s.boardEdit))
	mux.HandleFunc("POST /boards/{id}/edit", s.requireSysop(s.boardUpdate))
	mux.HandleFunc("POST /boards/{id}/delete", s.requireSysop(s.boardDelete))
	mux.HandleFunc("GET /aprs", s.requireAuth(s.aprs))
	mux.HandleFunc("GET /aprs/sent/{id}", s.requireAuth(s.aprsSentDetail))
	mux.HandleFunc("GET /aprs/received/{id}", s.requireAuth(s.aprsReceivedDetail))
	mux.HandleFunc("GET /aprs/received/{id}/reply", s.requireAuth(s.aprsReceivedReplyForm))
	mux.HandleFunc("POST /aprs/sent/delete", s.requireAuth(s.aprsSentBulkDelete))
	mux.HandleFunc("POST /aprs/sent/{id}/delete", s.requireAuth(s.aprsSentDelete))
	mux.HandleFunc("POST /aprs/received/delete", s.requireAuth(s.aprsReceivedBulkDelete))
	mux.HandleFunc("POST /aprs/received/{id}/delete", s.requireAuth(s.aprsReceivedDelete))
	mux.HandleFunc("POST /aprs/received/{id}/reply", s.requireAuth(s.aprsReceivedReply))
	mux.HandleFunc("POST /aprs/toggle", s.requireAuth(s.aprsToggle))
	mux.HandleFunc("POST /aprs/send", s.requireAuth(s.aprsSend))
	mux.HandleFunc("GET /admin/users", s.requireSysop(s.adminUsers))
	mux.HandleFunc("POST /admin/users/{callsign}", s.requireSysop(s.adminUserUpdate))
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *server) requireAuth(next func(http.ResponseWriter, *http.Request, *dbUser)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.currentUser(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r, user)
	}
}

func (s *server) requireSysop(next func(http.ResponseWriter, *http.Request, *dbUser)) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request, user *dbUser) {
		if !s.isSysop(user) {
			http.Error(w, "sysop access required", http.StatusForbidden)
			return
		}
		next(w, r, user)
	})
}

func (s *server) currentUser(r *http.Request) (*dbUser, error) {
	cookie, err := r.Cookie("bbs_session")
	if err != nil || cookie.Value == "" {
		return nil, errUnauthorized
	}
	s.mu.RLock()
	callsign := s.sessions[cookie.Value]
	s.mu.RUnlock()
	if callsign == "" {
		return nil, errUnauthorized
	}
	var user dbUser
	if err := s.db.First(&user, "callsign = ?", callsign).Error; err != nil {
		return nil, errUnauthorized
	}
	if user.Disabled {
		return nil, errUnauthorized
	}
	if s.cfg.sysops[user.Callsign] && !user.IsSysop {
		s.db.Model(&user).Update("is_sysop", true)
		user.IsSysop = true
	}
	return &user, nil
}

func (s *server) setSession(w http.ResponseWriter, callsign string) {
	token := randomToken(32)
	s.mu.Lock()
	s.sessions[token] = callsign
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "bbs_session", Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (s *server) clearSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("bbs_session"); err == nil {
		s.mu.Lock()
		delete(s.sessions, cookie.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "bbs_session", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func (s *server) view(w http.ResponseWriter, r *http.Request, name string, user *dbUser, data any, msg string, errText string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	lang := "en"
	if user != nil && user.Language != "" {
		lang = user.Language
	}
	vd := viewData{AppName: s.cfg.name, Location: s.cfg.location, Topic: s.cfg.topic, SysopName: s.cfg.sysopName, Path: r.URL.Path, User: user, Lang: lang, Flash: msg, Error: errText, Data: data}
	if user != nil {
		vd.IsSysop = s.isSysop(user)
	}
	if err := s.tpl.ExecuteTemplate(w, name, vd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
