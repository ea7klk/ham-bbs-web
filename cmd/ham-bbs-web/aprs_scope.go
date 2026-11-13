package main

import (
	"strings"
)

// APRS message sections.
//
// Version one classified by peer callsign alone: anything from ANSRVR was
// "#APRSThursday". Production traffic showed that wrong on the larger half of the
// data.
//
// ANSRVR is the *group server*, not a Thursday bot: it carries AARN, NEAPRS and
// every other group this station has joined. And the #APRSThursday conversation
// does not come from ANSRVR at all — it is relayed by the participants, each
// message tagged with the group in the body:
//
//	N:HOTG happy aprsthursday!!!        from KB3KBR-2
//	N:HOTG Saludos desde Republica...   from HI3RRR-7
//
// Measured on the live database: 478 messages carry the N:HOTG tag from 413
// distinct stations, and 6 came from ANSRVR. Peer-only classification put all 472
// in "other stations": the Thursday section was nearly empty while the net sat
// elsewhere.
//
// The rule, identical to the classic BBS's (bbs/cmd/bbs/aprs_scope.go):
//
//  1. A group tag in the body decides — "N:HOTG …" is Thursday, any other
//     "N:<GROUP> …" is group-server traffic.
//  2. Otherwise the peer decides: APRSPH is the net bot, ANSRVR the group server.
//  3. ANSRVR traffic that names HOTG without a tag (our own "CQ HOTG") is Thursday.
//
// An acknowledgement names no group, so it lands with the group server rather
// than being guessed into a net it may not belong to.
//
// Classification happens here, in Go, rather than in SQL. A SQL predicate would
// have to re-implement the tag parser — the same duplication that produced a real
// bug once, when a five-character tag limit silently dropped "NEAPRS" into the
// wrong section. A user's history is bounded by the classic BBS's retention (500
// received, 200 sent), so loading it and filtering here costs nothing and keeps
// one implementation of the rule per repository.

// aprsScope names one section of the APRS history.
type aprsScope string

const (
	// aprsScopeDirect is traffic with another station that is not group traffic.
	aprsScopeDirect aprsScope = "direct"
	// aprsScopeAPRSPH is the #APRSPH net bot.
	aprsScopeAPRSPH aprsScope = "aprsph"
	// aprsScopeThursday is the #APRSThursday net: group traffic tagged HOTG,
	// whichever station relayed it.
	aprsScopeThursday aprsScope = "thursday"
	// aprsScopeGroups is the rest of the group server's traffic — other groups,
	// and acknowledgements that name no group.
	aprsScopeGroups aprsScope = "groups"
)

// aprsScopes is the display order: what most callers want first, the vague bucket
// last.
var aprsScopes = []aprsScope{
	aprsScopeDirect, aprsScopeAPRSPH, aprsScopeThursday, aprsScopeGroups,
}

const (
	// aprsThursdayGroup is the ANSRVR group name for #APRSThursday.
	aprsThursdayGroup = "HOTG"
	// aprsGroupTagMaxLen bounds a group name in an "N:<GROUP>" tag. Group names
	// are short in practice — HOTG, AARN, NEAPRS — but this is a shape check, not
	// a membership test: a longer token is not a group tag.
	aprsGroupTagMaxLen = 8
)

// aprsGroupTag extracts the group from a group-relayed body.
//
// The tag is "N:", one to eight characters, then a space or the end. Anything
// that does not match that shape is not a tag, so ordinary text beginning with
// "N:" cannot masquerade as one.
func aprsGroupTag(text string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.ToUpper(strings.TrimSpace(text)), "N:")
	if !ok {
		return "", false
	}
	end := strings.IndexAny(rest, " {")
	if end < 0 {
		end = len(rest)
	}
	group := strings.TrimSpace(rest[:end])
	if group == "" || len(group) > aprsGroupTagMaxLen {
		return "", false
	}
	return group, true
}

// aprsMentionsThursday reports whether a body names the #APRSThursday group or
// net, which is how our own outgoing commands identify it: "CQ HOTG", "K HOTG".
func aprsMentionsThursday(text string) bool {
	upper := strings.ToUpper(text)
	return strings.Contains(upper, aprsThursdayGroup) || strings.Contains(upper, "APRSTHURSDAY")
}

// aprsScopeOfMessage classifies one stored message. peer is the other end of the
// conversation — the sender of a received message, the addressee of a sent one.
func aprsScopeOfMessage(peer, text string) aprsScope {
	if group, ok := aprsGroupTag(text); ok {
		if group == aprsThursdayGroup {
			return aprsScopeThursday
		}
		return aprsScopeGroups
	}
	switch baseCallsign(peer) {
	case aprsBotAPRSPH:
		return aprsScopeAPRSPH
	case aprsBotANSRVR:
		if aprsMentionsThursday(text) {
			return aprsScopeThursday
		}
		return aprsScopeGroups
	}
	return aprsScopeDirect
}

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
	case aprsScopeGroups:
		return aprsScopeGroups, true
	}
	return "", false
}

// baseCallsign drops the SSID from a callsign.
func baseCallsign(peer string) string {
	base := strings.ToUpper(strings.TrimSpace(normalizeAPRSCallsign(peer)))
	if i := strings.Index(base, "-"); i > 0 {
		return base[:i]
	}
	return base
}

// labelKey and descriptionKey name the translation keys for a section.
func (sc aprsScope) labelKey() string {
	switch sc {
	case aprsScopeAPRSPH:
		return "aprs_section_aprsph"
	case aprsScopeThursday:
		return "aprs_section_thursday"
	case aprsScopeGroups:
		return "aprs_section_groups"
	}
	return "aprs_section_direct"
}

func (sc aprsScope) descriptionKey() string {
	switch sc {
	case aprsScopeAPRSPH:
		return "aprs_section_aprsph_desc"
	case aprsScopeThursday:
		return "aprs_section_thursday_desc"
	case aprsScopeGroups:
		return "aprs_section_groups_desc"
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

// aprsSection is one tab: where it goes, what it says and how much is in it.
type aprsSection struct {
	Scope     aprsScope
	Label     string
	Count     int64
	URL       string
	Current   bool
	Direction string
}

// aprsSectionTabs builds the tab list for one list page from per-section counts.
func (s *server) aprsSectionTabs(lang, direction string, counts map[aprsScope]int64, current aprsScope) []aprsSection {
	tabs := make([]aprsSection, 0, len(aprsScopes))
	for _, scope := range aprsScopes {
		tabs = append(tabs, aprsSection{
			Scope:     scope,
			Label:     s.t(lang, scope.labelKey()),
			Count:     counts[scope],
			URL:       aprsListPath(direction, scope),
			Current:   scope == current,
			Direction: direction,
		})
	}
	return tabs
}
