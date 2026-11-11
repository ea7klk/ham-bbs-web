package main

import (
	"bytes"
	"log"
	"net/http"
)

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.requireAuth(s.home))
	// Public account pages. The tokenised links are what the SSH BBS mails, so
	// they must resolve without a session.
	mux.HandleFunc("GET /login", s.loginForm)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("GET /register", s.registerForm)
	mux.HandleFunc("POST /register", s.registerPost)
	mux.HandleFunc("GET /forgot", s.public(s.forgotForm))
	mux.HandleFunc("POST /forgot", s.public(s.forgotPost))
	mux.HandleFunc("GET /activate", s.public(s.activateForm))
	mux.HandleFunc("POST /activate", s.public(s.activatePost))
	mux.HandleFunc("GET /reset", s.public(s.resetForm))
	mux.HandleFunc("POST /reset", s.public(s.resetPost))
	mux.HandleFunc("GET /email/confirm", s.public(s.emailConfirm))

	// Design mockups: static, anonymous, and not part of the application.
	mockups, mockupErr := mockupHandler()
	if mockupErr != nil {
		// The mockups are embedded at build time, so this cannot fail in a
		// correctly built binary; say so rather than serving nothing silently.
		log.Printf("design mockups unavailable: %v", mockupErr)
	} else {
		mux.HandleFunc("GET /mockups", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/mockups/", http.StatusMovedPermanently)
		})
		mux.Handle("GET /mockups/", mockups)
	}
	mux.HandleFunc("POST /logout", s.logoutPost)
	mux.HandleFunc("POST /language", s.requireAuth(s.languagePost))
	mux.HandleFunc("GET /profile", s.requireAuth(s.profileForm))
	mux.HandleFunc("POST /profile", s.requireAuth(s.profilePost))
	mux.HandleFunc("GET /directory", s.requireAuth(s.directory))
	mux.HandleFunc("GET /about", s.requireAuth(s.about))
	mux.HandleFunc("GET /resources", s.requireAuth(s.resources))
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
	mux.HandleFunc("GET /messages/{id}/edit", s.requireSysop(s.messageEditForm))
	mux.HandleFunc("POST /messages/{id}/edit", s.requireSysop(s.messageUpdate))
	mux.HandleFunc("GET /boards/{id}/edit", s.requireSysop(s.boardEdit))
	mux.HandleFunc("POST /boards/{id}/edit", s.requireSysop(s.boardUpdate))
	mux.HandleFunc("POST /boards/{id}/delete", s.requireSysop(s.boardDelete))
	mux.HandleFunc("GET /aprs", s.requireAuth(s.aprs))

	// APRS bulletins are read-only: the receiver in the classic BBS owns the
	// table. They are global rather than per user, so any signed-in caller can
	// read them — EnableAPRS governs sending, not reading.
	mux.HandleFunc("GET /aprs/bulletins", s.requireAuth(s.aprsBulletinIndex))
	mux.HandleFunc("GET /aprs/bulletins/{category}", s.requireAuth(s.aprsBulletinCategory))
	mux.HandleFunc("GET /aprs/bulletins/{category}/{id}", s.requireAuth(s.aprsBulletinDetail))

	// The two bots: APRSPH's command set, and the #APRSThursday group commands.
	mux.HandleFunc("GET /aprs/aprsph", s.requireAuth(s.aprsphPage))
	mux.HandleFunc("POST /aprs/aprsph", s.requireAuth(s.aprsphSend))
	mux.HandleFunc("POST /aprs/thursday/join", s.requireAuth(s.thursdayJoin))
	mux.HandleFunc("POST /aprs/thursday/leave", s.requireAuth(s.thursdayLeave))
	mux.HandleFunc("POST /aprs/thursday/keepalive", s.requireAuth(s.thursdayKeepalive))
	mux.HandleFunc("GET /aprs/received", s.requireAuth(s.aprsReceived))
	mux.HandleFunc("GET /aprs/sent", s.requireAuth(s.aprsSent))
	mux.HandleFunc("GET /aprs/send", s.requireAuth(s.aprsSendForm))
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

// public serves a page that works with or without a session. A signed-in caller
// gets their language and their navigation; an anonymous one gets neither, which
// is what a mailed link needs.
func (s *server) public(next func(http.ResponseWriter, *http.Request, *dbUser)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.currentUser(r)
		if err != nil {
			user = nil
		}
		next(w, r, user)
	}
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
	s.viewStatus(w, r, http.StatusOK, name, user, data, msg, errText)
}

// viewStatus renders a page and then sends it with an explicit status.
//
// The template is executed into a buffer first. html/template writes as it goes
// and only reports an error at the end, so executing straight into the response
// would emit a truncated page and then be unable to change the status. Buffering
// means a template error becomes a plain 500 instead of half a page, and it is
// what lets a 404 be a real page rather than three words in the default font.
func (s *server) viewStatus(w http.ResponseWriter, r *http.Request, status int, name string, user *dbUser, data any, msg string, errText string) {
	lang := "en"
	if user != nil && user.Language != "" {
		lang = user.Language
	}
	vd := viewData{AppName: s.cfg.name, Location: s.cfg.location, Topic: s.cfg.topic, SysopName: s.cfg.sysopName, Path: r.URL.Path, User: user, Lang: lang, Flash: msg, Error: errText, Data: data}
	if user != nil {
		vd.IsSysop = s.isSysop(user)
	}
	var page bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&page, name, vd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = page.WriteTo(w)
}

// notFound renders the 404 page with the caller's own navigation, so a stale
// link leaves them somewhere they can move on from.
func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if err != nil {
		user = nil
	}
	s.viewStatus(w, r, http.StatusNotFound, "not_found", user, map[string]any{"Path": r.URL.Path}, "", "")
}
