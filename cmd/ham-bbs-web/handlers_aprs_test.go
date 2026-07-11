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
