package main

import (
	"bufio"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"os"
	"strings"

	"inet.af/netaddr"
)

type route uint8

const (
	haveRoute route = 1 << iota
	isTransit
	isGov
	onSIX
	reserved
	isHE
	isCloudflare
)

func (r route) color() color.NRGBA {
	if r&reserved != 0 {
		return color.NRGBA{140, 140, 140, 255}
	}
	if r&onSIX != 0 {
		if r&isGov != 0 {
			return color.NRGBA{161, 188, 237, 255}
		}
		return color.NRGBA{0, 44, 201, 255}
	}
	if r&isTransit != 0 {
		return color.NRGBA{244, 252, 3, 255}
	}
	if r&isCloudflare != 0 {
		return color.NRGBA{244, 129, 32, 255}
	}
	if r&isHE != 0 {
		return color.NRGBA{86, 232, 125, 255}
	}
	if r&haveRoute != 0 {
		return color.NRGBA{255, 0, 0, 255}
	}
	return color.NRGBA{235, 235, 247, 255}
}

type routeMap [1 << 24]route

func (m *routeMap) stats(skip route) (six24, reachable, total24 int) {
	for _, r := range m {
		if r&skip != 0 {
			continue
		}
		total24++
		if r&onSIX != 0 {
			six24++
		}
		if r&haveRoute != 0 {
			reachable++
		}
	}
	return
}

func (m *routeMap) set(ip netaddr.IP, bit route) {
	m[routeNum(ip)] |= bit
}

func (m *routeMap) setPrefix(p netaddr.IPPrefix, bit route) {
	if !p.IsValid() || p.Bits() > 24 || p.Bits() == 0 {
		return
	}
	r := p.Range()
	from, to := routeNum(r.From()), routeNum(r.To())
	for i := from; i <= to; i++ {
		m[i] |= bit
	}
}

func newRouteMap() *routeMap {
	m := new(routeMap)
	for _, s := range []string{
		"224.0.0.0/4", // multicast
		"240.0.0.0/4", // future use

		"0.0.0.0/8",
		"127.0.0.0/8",

		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",

		"100.64.0.0/10", // CGNAT

		"169.254.0.0/16", // link local

		"198.18.0.0/15", // benchmarking

	} {
		m.setPrefix(netaddr.MustParseIPPrefix(s), reserved)
	}
	return m
}

func routeNum(ip netaddr.IP) int {
	a4 := ip.As4()
	n := binary.BigEndian.Uint32(a4[:])
	return int(n >> 8)
}

func addRouteServers(rm *routeMap) {
	res, err := http.Get("https://www.seattleix.net/rs/rs2.1500.v4.unique.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		log.Fatal(res.Status)
	}
	bs := bufio.NewScanner(res.Body)
	bs.Scan() // skip first line
	for bs.Scan() {
		line := bs.Text()
		i := strings.Index(line, "via ")
		s := strings.TrimSpace(line[:i])
		ipp, err := netaddr.ParseIPPrefix(s)
		if err != nil {
			log.Fatalf("bogus line %q: %v", s, err)
		}
		bits := ipp.Bits()
		if bits > 24 {
			continue
		}
		if bits == 8 {
			// Turns out these are all US gov/DoD/army/etc stuff.
			rm.setPrefix(ipp, isGov)
		}
		rm.setPrefix(ipp, onSIX)
	}
}

func addReachable(rm *routeMap, filename string) {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	bs := bufio.NewScanner(f)
	for bs.Scan() {
		line := strings.TrimSpace(bs.Text())
		i := strings.Index(line, "via ")
		if i == -1 {
			continue
		}
		s := strings.TrimSpace(line[:i])
		if !strings.Contains(s, "/") {
			continue
		}
		ipp, err := netaddr.ParseIPPrefix(s)
		if err != nil {
			log.Fatalf("bogus line %q: %v", s, err)
		}
		bits := ipp.Bits()
		if bits > 24 {
			continue
		}
		rm.setPrefix(ipp, haveRoute)
	}
}

func addBirdRoutes(rm *routeMap, filename string) {
	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	bs := bufio.NewScanner(f)
	for bs.Scan() {
		line := strings.TrimSpace(bs.Text())
		if !strings.Contains(line, " * ") {
			continue
		}
		i := strings.Index(line, "via ")
		if i == -1 {
			continue
		}
		s := strings.TrimSpace(line[:i])
		ipp, err := netaddr.ParseIPPrefix(s)
		if err != nil || ipp.Bits() > 24 || ipp.Bits() == 0 {
			continue
		}
		var bits route
		switch {
		case strings.Contains(line, "[doof_transit "):
			bits = isTransit
		case strings.Contains(line, "[he "):
			bits = isHE
		case strings.Contains(line, "[cloudflare "):
			bits = isCloudflare
		}
		if bits != 0 {
			rm.setPrefix(ipp, bits)
		}
	}
}

var (
	route4    = flag.String("v4routes", "", "if non-empty, text file to Linux ip -4 route output to add; this flag is very particular to my setup. Modify code as needed for your setup.")
	routeBird = flag.String("bird-routes", "", "if non-empty, bird 'show routes' output to parse; this flag is very particular to my setup. Modify code as needed for your setup.")
)

func main() {
	flag.Parse()
	rm := newRouteMap()

	if *route4 != "" {
		addReachable(rm, *route4)
	}
	if *routeBird != "" {
		addBirdRoutes(rm, *routeBird)
	}
	addRouteServers(rm)

	six24, reachable, total24 := rm.stats(reserved)
	fmt.Printf("num /24s   six: %v of %v (%0.02f%%)\n", six24, total24, 100.0*float64(six24)/float64(total24))
	fmt.Printf("num /24s reach: %v of %v (%0.02f%%)\n", reachable, total24, 100.0*float64(reachable)/float64(total24))

	six24, _, total24 = rm.stats(reserved | isGov)
	fmt.Printf("num /24s non-gov: %v of %v (%0.02f%%)\n", six24, total24, 100.0*float64(six24)/float64(total24))

	log.Printf("making image..")
	im := image.NewNRGBA(image.Rect(0, 0, 1<<12, 1<<12))
	for i, r := range rm {
		x, y := hilbertXY(uint32(i))
		im.SetNRGBA(int(x), int(y), r.color())
	}
	pf, err := os.Create("map.png")
	if err != nil {
		panic(err)
	}
	log.Printf("encoding png..")
	if err := png.Encode(pf, im); err != nil {
		panic(err)
	}
	pf.Close()
}

// https://github.com/hrbrmstr/ipv4-heatmap/blob/master/hilbert.c
func hilbertXY(slash24Prefix uint32) (x, y uint32) {
	const order = 12 // 4096x4096 /24s
	var state uint32
	for i := 2*order - 2; i >= 0; i -= 2 {
		row := 4*state | ((slash24Prefix >> i) & 3)
		x = (x << 1) | ((0x936C >> row) & 1)
		y = (y << 1) | ((0x39C6 >> row) & 1)
		state = (0x3E6B94C1 >> (2 * row)) & 3
	}
	return
}
