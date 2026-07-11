package main

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *server) bulletins(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var items []dbBulletin
	s.db.Order("position, id").Find(&items)
	s.view(w, r, "bulletins", user, items, "", "")
}

func (s *server) bulletinNew(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "bulletin_form", user, dbBulletin{}, "", "")
}

func (s *server) bulletinCreate(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var maxPos int
	s.db.Model(&dbBulletin{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)
	item := dbBulletin{Position: maxPos + 1, Title: strings.TrimSpace(r.FormValue("title")), Body: strings.TrimSpace(r.FormValue("body")), Updated: now(), From: user.Callsign}
	if item.Title == "" || item.Body == "" {
		s.view(w, r, "bulletin_form", user, item, "", s.t(user.Language, "required"))
		return
	}
	s.db.Create(&item)
	s.logBBSAction(user.Callsign, "web_bulletin_create", "title=%q", item.Title)
	http.Redirect(w, r, "/bulletins", http.StatusSeeOther)
}

func (s *server) bulletinEdit(w http.ResponseWriter, r *http.Request, user *dbUser) {
	item, err := s.findBulletin(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.view(w, r, "bulletin_form", user, item, "", "")
}

func (s *server) bulletinUpdate(w http.ResponseWriter, r *http.Request, user *dbUser) {
	item, err := s.findBulletin(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	item.Title = strings.TrimSpace(r.FormValue("title"))
	item.Body = strings.TrimSpace(r.FormValue("body"))
	item.Updated = now()
	item.From = user.Callsign
	if item.Title == "" || item.Body == "" {
		s.view(w, r, "bulletin_form", user, item, "", s.t(user.Language, "required"))
		return
	}
	s.db.Save(&item)
	s.logBBSAction(user.Callsign, "web_bulletin_update", "title=%q", item.Title)
	http.Redirect(w, r, "/bulletins", http.StatusSeeOther)
}

func (s *server) bulletinDelete(w http.ResponseWriter, r *http.Request, user *dbUser) {
	item, err := s.findBulletin(r)
	if err == nil {
		s.db.Delete(&item)
		s.logBBSAction(user.Callsign, "web_bulletin_delete", "title=%q", item.Title)
	}
	http.Redirect(w, r, "/bulletins", http.StatusSeeOther)
}

func (s *server) findBulletin(r *http.Request) (dbBulletin, error) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var item dbBulletin
	return item, s.db.First(&item, id).Error
}
