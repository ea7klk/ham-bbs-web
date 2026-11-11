package main

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// APRS message sections.
//
// Bot traffic used to share the list with messages from other operators, so
// somebody looking for a reply from a station had to page past net check-ins,
// acknowledgements and group notices to find it. The two bots are identifiable
// from the stored row alone — the peer *is* the bot: a sent row addresses it, a
// received row comes from it.
//
// The split is by peer, not by content. An acknowledgement from ANSRVR
// ("ack1B}") carries no group name, so a content rule would file it with the
// wrong traffic and hide it from the message it acknowledges.
//
// The rule is the classic BBS's (bbs/cmd/bbs/aprs_scope.go). It has to be: both
// applications show the same rows, and a message filed in one section here and
// another there would be a bug with no single owner.

// aprsScope names one section of the APRS history.
type aprsScope string

const (
	// aprsScopeDirect is traffic with another station: everything not addressed
	// to, or sent by, a bot.
	aprsScopeDirect aprsScope = "direct"
	// aprsScopeAPRSPH is the #APRSPH net bot.
	aprsScopeAPRSPH aprsScope = "aprsph"
	// aprsScopeThursday is the ANSRVR group server, which carries #APRSThursday
	// and any other group the station has joined.
	aprsScopeThursday aprsScope = "thursday"
)

// aprsScopes is the display order, direct first because it is what most callers
// are looking for.
var aprsScopes = []aprsScope{aprsScopeDirect, aprsScopeAPRSPH, aprsScopeThursday}

// aprsScopeOfParam maps the ?section= value to a section. An absent value is the
// direct section, so /aprs/received keeps working as it did.
func aprsScopeOfParam(value string) (aprsScope, bool) {
	switch aprsScope(strings.TrimSpace(value)) {
	case "", aprsScopeDirect:
		return aprsScopeDirect, true
	case aprsScopeAPRSPH:
		return aprsScopeAPRSPH, true
	case aprsScopeThursday:
		return aprsScopeThursday, true
	}
	return "", false
}

// aprsScopeOfPeer classifies a peer callsign-SSID.
//
// The SSID is ignored: the bots answer from their base callsign, but a station
// may address APRSPH-0 and an acknowledgement can arrive from ANSRVR with any
// SSID.
func aprsScopeOfPeer(peer string) aprsScope {
	return aprsScopeOfBase(baseCallsign(peer))
}

// baseCallsign drops the SSID from a callsign.
func baseCallsign(peer string) string {
	base := strings.ToUpper(strings.TrimSpace(normalizeAPRSCallsign(peer)))
	if i := strings.Index(base, "-"); i > 0 {
		return base[:i]
	}
	return base
}

// aprsScopeOfBase classifies an already-normalised base callsign.
func aprsScopeOfBase(base string) aprsScope {
	switch base {
	case aprsBotAPRSPH:
		return aprsScopeAPRSPH
	case aprsBotANSRVR:
		return aprsScopeThursday
	}
	return aprsScopeDirect
}

// labelKey and descriptionKey name the translation keys for a section.
func (sc aprsScope) labelKey() string {
	switch sc {
	case aprsScopeAPRSPH:
		return "aprs_section_aprsph"
	case aprsScopeThursday:
		return "aprs_section_thursday"
	}
	return "aprs_section_direct"
}

func (sc aprsScope) descriptionKey() string {
	switch sc {
	case aprsScopeAPRSPH:
		return "aprs_section_aprsph_desc"
	case aprsScopeThursday:
		return "aprs_section_thursday_desc"
	}
	return "aprs_section_direct_desc"
}

// queryValue is the ?section= value for a section; the direct section has none,
// so its URLs stay the plain ones.
func (sc aprsScope) queryValue() string {
	if sc == aprsScopeDirect {
		return ""
	}
	return string(sc)
}

// bots are the base callsigns this section is built from.
func (sc aprsScope) bots() []string {
	switch sc {
	case aprsScopeAPRSPH:
		return []string{aprsBotAPRSPH}
	case aprsScopeThursday:
		return []string{aprsBotANSRVR}
	}
	return []string{aprsBotAPRSPH, aprsBotANSRVR}
}

// scopeWhere applies a section filter over a peer column.
//
// It is applied in SQL rather than after loading, because filtering a page after
// pagination would leave gaps: a page with two hidden rows shows two fewer, and
// the totals would count messages the caller cannot see.
//
// The match is on the base callsign, mirroring aprsScopeOfPeer. SQLite has no
// "strip after the dash" operator, so a peer matches the bot's exact callsign or
// any "-SSID" form of it.
func (sc aprsScope) scopeWhere(query *gorm.DB, column string) *gorm.DB {
	// The column is quoted: `to` is an SQLite keyword and an unquoted use in a
	// function call is a syntax error.
	quoted := `"` + column + `"`
	exact := func(bot string) string {
		return fmt.Sprintf("(UPPER(%[1]s) = ? OR UPPER(%[1]s) LIKE ?)", quoted)
	}
	// LIKE with a trailing "-%" matches any SSID. The underscore and percent in a
	// callsign cannot occur — APRSCallsignRE is letters, digits and a dash — so
	// no escaping is needed.
	withSSID := func(bot string) string { return bot + "-%" }

	switch sc {
	case aprsScopeAPRSPH:
		return query.Where(exact(column), aprsBotAPRSPH, withSSID(aprsBotAPRSPH))
	case aprsScopeThursday:
		return query.Where(exact(column), aprsBotANSRVR, withSSID(aprsBotANSRVR))
	}
	return query.Where(
		"NOT ("+exact(column)+") AND NOT ("+exact(column)+")",
		aprsBotAPRSPH, withSSID(aprsBotAPRSPH), aprsBotANSRVR, withSSID(aprsBotANSRVR),
	)
}

// aprsScopeCounts counts each section for one direction, for the tab bar.
func (s *server) aprsScopeCounts(callsign string, sent bool) (map[aprsScope]int64, error) {
	callsign = normalizeCallsign(callsign)
	counts := map[aprsScope]int64{}
	for _, scope := range aprsScopes {
		var total int64
		var err error
		if sent {
			err = scope.scopeWhere(s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", callsign), "to").Count(&total).Error
		} else {
			err = scope.scopeWhere(s.db.Model(&dbAPRSReceived{}).Where("user_callsign = ?", callsign), "from").Count(&total).Error
		}
		if err != nil {
			return nil, err
		}
		counts[scope] = total
	}
	return counts, nil
}

// aprsSection is one tab: where it goes, what it says and how much is in it.
type aprsSection struct {
	Scope   aprsScope
	Label   string
	Count   int64
	URL     string
	Current bool
	// Direction is "received" or "sent", so the template can build a link back to
	// the other direction in the same section.
	Direction string
}

// aprsSectionTabs builds the tab list for one list page.
func (s *server) aprsSectionTabs(lang, direction, callsign string, current aprsScope) ([]aprsSection, error) {
	counts, err := s.aprsScopeCounts(callsign, direction == "sent")
	if err != nil {
		return nil, err
	}
	tabs := make([]aprsSection, 0, len(aprsScopes))
	for _, scope := range aprsScopes {
		url := "/aprs/" + direction
		if value := scope.queryValue(); value != "" {
			url += "?section=" + value
		}
		tabs = append(tabs, aprsSection{
			Scope:     scope,
			Label:     s.t(lang, scope.labelKey()),
			Count:     counts[scope],
			URL:       url,
			Current:   scope == current,
			Direction: direction,
		})
	}
	return tabs, nil
}
