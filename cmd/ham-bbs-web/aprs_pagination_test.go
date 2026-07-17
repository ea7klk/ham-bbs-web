package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPRSPageParsingAndPagination(t *testing.T) {
	for _, test := range []struct {
		query       string
		wantPage    int
		wantPerPage int
	}{
		{"", 1, 10},
		{"?page=3&per_page=25", 3, 25},
		{"?page=0&per_page=50", 1, 50},
		{"?page=bad&per_page=15", 1, 10},
	} {
		r := httptest.NewRequest(http.MethodGet, "/aprs/sent"+test.query, nil)
		page, perPage := parseAPRSPage(r)
		if page != test.wantPage || perPage != test.wantPerPage {
			t.Fatalf("parseAPRSPage(%q) = %d/%d, want %d/%d", test.query, page, perPage, test.wantPage, test.wantPerPage)
		}
	}

	pagination := newAPRSPagination("/aprs/sent", 8, 25, 51)
	if pagination.Page != 3 || pagination.Pages != 3 || !pagination.HasPrevious || pagination.HasNext || pagination.PreviousURL != "/aprs/sent?page=2&per_page=25" {
		t.Fatalf("clamped pagination = %#v", pagination)
	}
	empty := newAPRSPagination("/aprs/received", 2, 10, 0)
	if empty.Page != 1 || empty.Pages != 1 || empty.HasPrevious || empty.HasNext || len(empty.PageSizes) != 3 {
		t.Fatalf("empty pagination = %#v", empty)
	}
}

func TestAPRSPagedQueriesAndTimestampFormatting(t *testing.T) {
	s := newTestServer(t)
	for i := 0; i < 23; i++ {
		if err := s.db.Create(&dbAPRSSent{UserCallsign: "EA7KLK", Position: i, At: "2026-07-17 12:00", Text: "sent"}).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.db.Create(&dbAPRSReceived{UserCallsign: "EA7KLK", Position: i, At: "2026-07-17 12:00", Text: "received"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.Create(&dbAPRSSent{UserCallsign: "OTHER", Position: 999, Text: "private"}).Error; err != nil {
		t.Fatal(err)
	}

	sent, sentPage := s.sentAPRSPage("ea7klk", 2, 10, "/aprs/sent")
	if len(sent) != 10 || sentPage.Total != 23 || sentPage.Pages != 3 || sent[0].Position != 12 {
		t.Fatalf("sent page = %d/%#v, want 10 rows starting at position 12", sentPage.Total, sent)
	}
	received, receivedPage := s.receivedAPRSPage("EA7KLK", 3, 10, "/aprs/received")
	if len(received) != 3 || receivedPage.Total != 23 || received[0].Position != 2 {
		t.Fatalf("received page = %d/%#v, want 3 rows starting at position 2", receivedPage.Total, received)
	}
	dateTime := formatAPRSDateTime(" 2026-07-17 12:34:56 UTC ")
	if dateTime.Date != "2026-07-17" || dateTime.Time != "12:34:56 UTC" {
		t.Fatalf("formatAPRSDateTime() = %#v", dateTime)
	}
	if got := formatAPRSDateTime(""); got.Date != "" || got.Time != "" {
		t.Fatalf("formatAPRSDateTime(empty) = %#v", got)
	}
}
