package main

import (
	"sort"
	"strings"
	"time"
)

func normalizeCallsign(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func normalizeLocator(locator string) string {
	locator = strings.TrimSpace(locator)
	if len(locator) < 2 {
		return strings.ToUpper(locator)
	}
	return strings.ToUpper(locator[:2]) + locator[2:]
}

func boardID(name string) string {
	id := strings.Trim(boardIDRE.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if id == "" {
		return defaultBoardID
	}
	if len(id) > 40 {
		return id[:40]
	}
	return id
}

func now() string {
	return time.Now().UTC().Format("2006-01-02 15:04 UTC")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func singleLine(text string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")
}

func asciiSafe(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 32 && r <= 126 {
			b.WriteRune(r)
		} else {
			b.WriteRune('?')
		}
	}
	return b.String()
}

func sortUsers(users []dbUser) []dbUser {
	sort.Slice(users, func(i, j int) bool { return users[i].Callsign < users[j].Callsign })
	return users
}
