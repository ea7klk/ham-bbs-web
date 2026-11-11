package main

import (
	"fmt"
	"time"
)

// APRS bulletins, read-only.
//
// A bulletin is a message-format packet whose addressee is `BLNn` plus a group,
// not a callsign:
//
//	:BLN1NWS  :Tornado warning until 21:00Z
//	 ^^^ ^^^^
//	 slot group
//
// The classic SSH BBS receiver parses and stores them in db_aprs_bulletins,
// deduplicated by content because senders repeat the same bulletin every few
// minutes. This file reads that table; nothing here writes to it, because a
// second writer would mean two places deciding what a bulletin is.

const (
	// aprsBulletinRetention is how long a bulletin stays readable, measured from
	// the last time it was heard so one still being transmitted does not expire.
	// It must match the receiver's window or a row could be shown here after it
	// has been pruned there, or hidden while it still exists.
	aprsBulletinRetention = 10 * 24 * time.Hour
	// aprsBulletinStamp is the timestamp format both applications write.
	aprsBulletinStamp = "2006-01-02 15:04 UTC"
)

// aprsBulletinCategories lists the categories in display order — what a reader
// most needs first, and `other` last because it is the fallback for a group this
// BBS does not recognise.
//
// The ids and this order are the receiver's; the labels and descriptions come
// from translations.json, so the two surfaces say the same thing.
var aprsBulletinCategories = []string{
	"weather", "emergency", "traffic", "nets", "events", "dx",
	"satellite", "network", "service", "organisation", "test", "general", "other",
}

// aprsBulletinCategoryKnown reports whether a category id is one this BBS knows,
// so a row written by a newer receiver cannot invent a page.
func aprsBulletinCategoryKnown(category string) bool {
	for _, id := range aprsBulletinCategories {
		if id == category {
			return true
		}
	}
	return false
}

// aprsBulletinCutoff is the oldest last-seen stamp still inside the window.
func aprsBulletinCutoff(at time.Time) string {
	return at.Add(-aprsBulletinRetention).Format(aprsBulletinStamp)
}

// loadAPRSBulletins returns the live bulletins, newest first, optionally
// restricted to one category.
func (s *server) loadAPRSBulletins(category string, at time.Time) ([]dbAPRSBulletin, error) {
	query := s.db.Where("last_seen >= ?", aprsBulletinCutoff(at))
	if category != "" {
		query = query.Where("category = ?", category)
	}
	rows := []dbAPRSBulletin{}
	if err := query.Order("last_seen DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	// Third-party content, written by the receiver and displayed here. Clean it
	// on read for the same reason every other page does: a row from an older
	// version, or a client that bypassed the write guard, must not reach a
	// browser or a terminal unfiltered.
	for i := range rows {
		rows[i].Text = sanitizeMultilineText(rows[i].Text)
		rows[i].From = sanitizeSingleLineText(rows[i].From)
		rows[i].Group = sanitizeSingleLineText(rows[i].Group)
		rows[i].Raw = sanitizeSingleLineText(rows[i].Raw)
	}
	return rows, nil
}

// aprsBulletinCounts returns how many live bulletins each category holds.
func (s *server) aprsBulletinCounts(at time.Time) (map[string]int, error) {
	type row struct {
		Category string
		Count    int
	}
	rows := []row{}
	err := s.db.Model(&dbAPRSBulletin{}).
		Select("category, count(*) as count").
		Where("last_seen >= ?", aprsBulletinCutoff(at)).
		Group("category").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, r := range rows {
		out[r.Category] = r.Count
	}
	return out, nil
}

// findAPRSBulletin returns one live bulletin by id.
func (s *server) findAPRSBulletin(id uint, at time.Time) (dbAPRSBulletin, bool) {
	row := dbAPRSBulletin{}
	err := s.db.Where("id = ? AND last_seen >= ?", id, aprsBulletinCutoff(at)).Take(&row).Error
	if err != nil {
		return dbAPRSBulletin{}, false
	}
	row.Text = sanitizeMultilineText(row.Text)
	row.From = sanitizeSingleLineText(row.From)
	row.Group = sanitizeSingleLineText(row.Group)
	row.Raw = sanitizeSingleLineText(row.Raw)
	return row, true
}

// bulletinAge renders how long ago a bulletin was last heard, in the same compact
// form the terminal uses: "now", "7m", "3h", "2d".
//
// It is relative rather than absolute because the question a reader has is "is
// this current?", and a full timestamp makes them do the subtraction. The exact
// stamps are on the detail page.
func bulletinAge(stamp string, at time.Time) string {
	when, err := time.Parse(aprsBulletinStamp, stamp)
	if err != nil {
		return ""
	}
	d := at.Sub(when)
	switch {
	case d < 0:
		// A clock skew between the receiver and this process should not read as
		// an age from the future.
		return "now"
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
