package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// aprsBulletinIndex lists the categories that currently hold something, with a
// count and a description. Categories with nothing in them are omitted, so the
// page reflects what has been heard rather than the whole taxonomy.
func (s *server) aprsBulletinIndex(w http.ResponseWriter, r *http.Request, user *dbUser) {
	at := time.Now()
	counts, err := s.aprsBulletinCounts(at)
	if err != nil {
		s.view(w, r, "aprs_bulletins", user, map[string]any{}, "", err.Error())
		return
	}
	type entry struct {
		ID    string
		Label string
		Desc  string
		Count int
	}
	entries := []entry{}
	for _, id := range aprsBulletinCategories {
		if counts[id] == 0 {
			continue
		}
		entries = append(entries, entry{
			ID:    id,
			Label: s.t(user.Language, "aprs_bulletin_category_"+id),
			Desc:  s.t(user.Language, "aprs_bulletin_desc_"+id),
			Count: counts[id],
		})
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	s.view(w, r, "aprs_bulletins", user, map[string]any{"Categories": entries, "Total": total}, "", "")
}

// aprsBulletinCategory lists the live bulletins in one category, newest first.
func (s *server) aprsBulletinCategory(w http.ResponseWriter, r *http.Request, user *dbUser) {
	category := strings.TrimSpace(r.PathValue("category"))
	if !aprsBulletinCategoryKnown(category) {
		s.notFound(w, r)
		return
	}
	at := time.Now()
	rows, err := s.loadAPRSBulletins(category, at)
	if err != nil {
		s.view(w, r, "aprs_bulletin_list", user, map[string]any{"Category": category}, "", err.Error())
		return
	}
	type row struct {
		Item dbAPRSBulletin
		Age  string
	}
	out := make([]row, 0, len(rows))
	for _, item := range rows {
		out = append(out, row{Item: item, Age: bulletinAge(item.LastSeen, at)})
	}
	s.view(w, r, "aprs_bulletin_list", user, map[string]any{
		"Category":  category,
		"Label":     s.t(user.Language, "aprs_bulletin_category_"+category),
		"Desc":      s.t(user.Language, "aprs_bulletin_desc_"+category),
		"Bulletins": out,
	}, "", "")
}

// aprsBulletinDetail shows one bulletin with its provenance.
func (s *server) aprsBulletinDetail(w http.ResponseWriter, r *http.Request, user *dbUser) {
	category := strings.TrimSpace(r.PathValue("category"))
	if !aprsBulletinCategoryKnown(category) {
		s.notFound(w, r)
		return
	}
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	item, ok := s.findAPRSBulletin(uint(id), time.Now())
	if !ok || item.Category != category {
		s.notFound(w, r)
		return
	}
	s.view(w, r, "aprs_bulletin_detail", user, map[string]any{
		"Item":         item,
		"Category":     category,
		"CategoryName": s.t(user.Language, "aprs_bulletin_category_"+category),
	}, "", "")
}
