package main

import (
	"testing"

	"inet.af/netaddr"
)

func TestRouteNum(t *testing.T) {
	tests := []struct {
		ip   string
		want int
	}{
		{"0.0.0.0", 0},
		{"0.0.1.1", 1},
		{"0.0.1.255", 1},
		{"0.0.2.255", 2},
		{"255.255.255.0", 1<<24 - 1},
	}
	for _, tt := range tests {
		ip := netaddr.MustParseIP(tt.ip)
		got := routeNum(ip)
		if got != tt.want {
			t.Errorf("routeNum(%q) = %v; want %v", tt.ip, got, tt.want)
		}
	}
}

func TestRouteMapSet(t *testing.T) {
	var m routeMap
	m.setPrefix(netaddr.MustParseIPPrefix("0.0.0.0/24"), 1)
	if m[0] != 1 {
		t.Errorf("not 1")
	}
	if m[1] != 0 {
		t.Errorf("not 0")
	}

}
