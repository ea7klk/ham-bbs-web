package main

import (
	"math"
	"testing"
)

func TestMaidenheadCenter(t *testing.T) {
	lat, lon, err := maidenheadCenter("im77ah")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(lat-37.3125) > 0.000001 {
		t.Fatalf("maidenheadCenter() lat = %.8f, want 37.3125", lat)
	}
	if math.Abs(lon-(-5.9583333333)) > 0.000001 {
		t.Fatalf("maidenheadCenter() lon = %.8f, want -5.95833333", lon)
	}
}

func TestFormatAPRSPositionBeaconPacket(t *testing.T) {
	got := formatAPRSPosition(37.3125, -5.9583333333, '\\', 'm', "HamNet BBS")
	want := `!3718.75N\00557.50WmHamNet BBS`
	if got != want {
		t.Fatalf("formatAPRSPosition() = %q, want %q", got, want)
	}

	got = formatAPRSBeaconPacket("ea7klk-0", 37.3125, -5.9583333333, "HamNet BBS")
	want = `EA7KLK-0>APRS,TCPIP*:!3718.75N\00557.50WmHamNet BBS`
	if got != want {
		t.Fatalf("formatAPRSBeaconPacket() = %q, want %q", got, want)
	}
}

func TestAPRSOutgoingPacketsForceSenderSSID0(t *testing.T) {
	got := formatAPRSMessagePacket("ea7klk-7", "ea1abc-9", "Hello")
	want := "EA7KLK-0>APRS,TCPIP*::EA1ABC-9 :Hello"
	if got != want {
		t.Fatalf("formatAPRSMessagePacket() = %q, want %q", got, want)
	}

	got = formatAPRSBeaconPacket("ea7klk-7", 37.3125, -5.9583333333, "HamNet BBS")
	want = `EA7KLK-0>APRS,TCPIP*:!3718.75N\00557.50WmHamNet BBS`
	if got != want {
		t.Fatalf("formatAPRSBeaconPacket() = %q, want %q", got, want)
	}
	if got, want := aprsSSID0("ea7klk-7"), "EA7KLK-0"; got != want {
		t.Fatalf("aprsSSID0(user callsign) = %q, want %q", got, want)
	}
}

func TestComposeAPRSDestinationPreservesUserSSIDChoice(t *testing.T) {
	if got, want := composeAPRSDestination("ea1abc", ""), "EA1ABC"; got != want {
		t.Fatalf("composeAPRSDestination(blank ssid) = %q, want %q", got, want)
	}
	if got, want := composeAPRSDestination("ea1abc", "0"), "EA1ABC-0"; got != want {
		t.Fatalf("composeAPRSDestination(default ssid) = %q, want %q", got, want)
	}
	if got, want := composeAPRSDestination("ea1abc", "7"), "EA1ABC-7"; got != want {
		t.Fatalf("composeAPRSDestination(custom ssid) = %q, want %q", got, want)
	}
	if got := composeAPRSDestination("", "7"); got != "" {
		t.Fatalf("composeAPRSDestination(empty callsign) = %q, want empty", got)
	}
}

func TestReceivedAPRSDetailTextCleanup(t *testing.T) {
	text := singleLineAPRSDetail(stripAPRSMessageID("one two\nthree{A12"))
	if text != "one two three" {
		t.Fatalf("clean received APRS text = %q, want %q", text, "one two three")
	}
	raw := singleLineAPRSDetail("raw1\nraw2")
	if raw != "raw1 raw2" {
		t.Fatalf("clean received APRS raw = %q, want %q", raw, "raw1 raw2")
	}
}

func TestParseAPRSIDs(t *testing.T) {
	got := parseAPRSIDs([]string{"3", "1", "3", "invalid", "0"})
	want := []uint{3, 1}
	if len(got) != len(want) {
		t.Fatalf("parseAPRSIDs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseAPRSIDs() = %v, want %v", got, want)
		}
	}
}

func TestTemplatesParse(t *testing.T) {
	parseTemplates(map[string]map[string]any{"en": {}})
}
