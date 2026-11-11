package main

import (
	"fmt"
	"strings"
	"time"
)

// Bot commands.
//
// Two addressed bots, and the difference between them is the thing that used to
// break this in the terminal client, so it is worth stating plainly:
//
//   - APRSPH is a station at the callsign APRSPH. It offers the #APRSPH net's
//     command set and answers every message.
//   - ANSRVR is the APRS group server. It is addressed directly and the group is
//     named in the text: "CQ HOTG" joins #APRSThursday, "U HOTG" leaves it,
//     "K HOTG" extends the subscription.
//
// Commands are APRS protocol, not prose: they are never translated, and they are
// never split across messages. The splitter prefixes every part with "[n/total] ",
// so a two-part "CQ HOTG <text>" would put the command in part one as group
// "[1/2]" and the text in part two as group "[2/2]" — the command would never
// reach the group it names. A command that does not fit is refused instead.

const (
	// aprsCommandIDReserve is the space the trailing {XXXXX message ID takes
	// inside the 67-character APRS message field, so a one-message command body
	// is limited to the remainder.
	aprsCommandIDReserve = 6
	// aprsCommandMaxBody is the longest body a bot command may carry and still
	// fit in a single APRS message.
	aprsCommandMaxBody = aprsMessageLimit - aprsCommandIDReserve

	// The two bots, by callsign.
	aprsBotAPRSPH = "APRSPH"
	aprsBotANSRVR = "ANSRVR"
	// aprsGroupHOTG is the #APRSThursday group on ANSRVR.
	aprsGroupHOTG = "HOTG"

	// ANSRVR commands, and the three #APRSThursday forms.
	aprsCommandCQ = "CQ"
	aprsCommandU  = "U"
	aprsCommandK  = "K"

	// APRSPH commands. Each is the protocol keyword the bot documents.
	aprsphCmdCQ       = "CQ"
	aprsphCmdNET      = "NET"
	aprsphCmdLIST     = "LIST"
	aprsphCmdLAST     = "LAST"
	aprsphCmdMINE     = "MINE"
	aprsphCmdSEARCH   = "SEARCH"
	aprsphCmdPath     = "?APRST"
	aprsphCmdPosition = "?APRSP"
	aprsphCmdStatus   = "?APRSS"
	aprsphCmdInbox    = "?APRSM"
	aprsphCmdCheckout = "U"
	aprsphCmdHelp     = "HELP"
	// aprsphCmdThursday selects the #APRSThursday log for LAST and MINE.
	aprsphCmdThursday = "THURS"
)

// aprsHOTGJoin, aprsHOTGLeave and aprsHOTGKeep are the three #APRSThursday
// commands in full.
const (
	aprsHOTGJoin  = aprsCommandCQ + " " + aprsGroupHOTG
	aprsHOTGLeave = aprsCommandU + " " + aprsGroupHOTG
	aprsHOTGKeep  = aprsCommandK + " " + aprsGroupHOTG
)

// errCommandTooLong reports a command that would have to be split.
type errCommandTooLong struct {
	max int
	got int
}

func (e errCommandTooLong) Error() string {
	return fmt.Sprintf("command is %d characters, the limit is %d: a command is never split", e.got, e.max)
}

// sendAPRSCommand sends text as exactly one APRS message, or refuses.
//
// This is the only way a bot command leaves the web app. `sendAPRSMessage` splits
// long text into numbered parts, which is right for ordinary messages and wrong
// for a command, so commands go through here instead.
func (s *server) sendAPRSCommand(source, destination, text string) (sentAPRS, error) {
	body := cleanAPRSBody(text)
	if len([]rune(body)) > aprsCommandMaxBody {
		return sentAPRS{At: now(), Status: "failed", Text: body,
				Parts: []sentAPRSPart{{Number: 1, Text: body, Status: "failed", Detail: "command too long"}}},
			errCommandTooLong{max: aprsCommandMaxBody, got: len([]rune(body))}
	}
	if body == "" {
		return sentAPRS{At: now(), Status: "failed"}, fmt.Errorf("empty command")
	}
	if !validAPRSCallsign(source) {
		return sentAPRS{At: now(), Status: "failed"}, fmt.Errorf("invalid source callsign")
	}
	if !validAPRSCallsign(destination) {
		return sentAPRS{At: now(), Status: "failed"}, fmt.Errorf("invalid destination callsign")
	}

	from := aprsSSID0(source)
	to := normalizeAPRSCallsign(destination)
	passcode := aprsPasscode(from)
	sent := sentAPRS{At: now(), From: from, To: to, Text: body, Status: "sent", Passcode: passcode}
	ids := aprsMessageIDs(from, to, 1, time.Now())
	sent.Parts, _ = s.sendAPRSParts(from, passcode, to, []string{body}, ids)
	for _, part := range sent.Parts {
		if normalizeAPRSStatus(part.Status) != "sent" {
			sent.Status = "failed"
			return sent, fmt.Errorf("%s", part.Detail)
		}
	}
	if len(sent.Parts) == 0 {
		sent.Status = "failed"
		return sent, fmt.Errorf("no part status returned")
	}
	return sent, nil
}

// aprsphCommand builds an APRSPH command from its parts.
//
// It mirrors the terminal client's builder exactly, because these strings are a
// wire format: a typo here does not fail, it silently does nothing on the air. It
// is a pure function so the mapping can be asserted without sending anything.
//
// count is "5", "10" or "15". The bot accepts a bare command (five messages) or a
// 10 / 15 suffix, so "5" adds nothing.
func aprsphCommand(kind, count, target, phrase string, thursday bool) string {
	target = strings.ToUpper(strings.TrimSpace(target))
	phrase = strings.TrimSpace(phrase)

	switch kind {
	case "last":
		command := aprsphCmdLAST + countSuffix(count)
		if thursday {
			command += " " + aprsphCmdThursday
		}
		return command
	case "mine":
		// The terminal client sends either the #APRSThursday log or a target
		// callsign, never both: "MINE THURS" and "MINE EA7KLK" are different
		// questions, and the bot reads the next token as one or the other.
		command := aprsphCmdMINE + countSuffix(count)
		if thursday {
			command += " " + aprsphCmdThursday
		} else if target != "" {
			command += " " + target
		}
		return command
	case "search":
		command := aprsphCmdSEARCH + countSuffix(count)
		if phrase != "" {
			command += " " + phrase
		}
		return command
	case "inbox":
		if target == "" {
			return aprsphCmdInbox
		}
		return aprsphCmdInbox + " " + target
	case "checkin":
		if phrase == "" {
			return aprsphCmdCQ
		}
		return aprsphCmdCQ + " " + phrase
	case "quiet":
		if phrase == "" {
			return aprsphCmdNET
		}
		return aprsphCmdNET + " " + phrase
	case "checkout":
		if phrase == "" {
			return aprsphCmdCheckout
		}
		return aprsphCmdCheckout + " " + phrase
	case "list":
		return aprsphCmdLIST
	case "path":
		return aprsphCmdPath
	case "position":
		return aprsphCmdPosition
	case "status":
		return aprsphCmdStatus
	case "help":
		return aprsphCmdHelp
	}
	return ""
}

// countSuffix maps a count to the bot's suffix. Five is the bare command.
func countSuffix(count string) string {
	switch count {
	case "10":
		return "10"
	case "15":
		return "15"
	default:
		return ""
	}
}

// aprsClock is the source of "now" for the weekday-sensitive paths. It is a var
// so a test can be deterministic: a gate that only fires six days out of seven is
// a gate whose test would otherwise skip on the seventh and prove nothing.
var aprsClock = time.Now

// aprsThursdayActive reports whether #APRSThursday is running, which is Thursdays
// 0000Z-2359Z.
func aprsThursdayActive(at time.Time) bool {
	return at.UTC().Weekday() == time.Thursday
}
