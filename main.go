package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"internet-notify/notify"
)

var (
	initialOnly          bool
	tick                 time.Duration
	connectivityEndpoint string
	ipEndpoint           string
	geoEndpoint          string
	notificationExpiry   time.Duration
)

type GeoInfo struct {
	Status  string `json:"status"`
	Country string `json:"country"`
}

type State struct {
	HTTPClient http.Client
	Connected  bool
	PublicIP   string
	Geo        *GeoInfo
}

func InitState() *State {
	transport := &http.Transport{
		DisableKeepAlives: true,
	}
	client := http.Client{Timeout: 5 * time.Second, Transport: transport}
	return &State{HTTPClient: client}
}

func (s *State) QueryConnectivity() {
	timeout := 5 * time.Second
	conn, err := net.DialTimeout("tcp", connectivityEndpoint, timeout)
	if err != nil {
		s.Connected = false
	} else {
		s.Connected = true
		conn.Close()
	}
}

func (s *State) QueryPublicIP() {
	if !s.Connected {
		s.PublicIP = ""
		return
	}
	resp, err := s.HTTPClient.Get(ipEndpoint)
	if err != nil {
		s.PublicIP = ""
		return
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		s.PublicIP = ""
		return
	}
	s.PublicIP = strings.TrimSpace(string(ip))
}

func (s *State) QueryGeoInfo() {
	if !s.Connected || s.PublicIP == "" {
		s.Geo = nil
		return
	}
	url := geoEndpoint + s.PublicIP
	resp, err := s.HTTPClient.Get(url)
	if err != nil {
		s.Geo = nil
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.Geo = nil
		return
	}
	var geo GeoInfo
	if err := json.Unmarshal(data, &geo); err != nil || geo.Status != "success" {
		s.Geo = nil
		return
	}
	s.Geo = &geo
}

func main() {

	flag.BoolVar(&initialOnly, "initialOnly", false, "Exit after sending the initial notification.")
	flag.DurationVar(&tick, "tick", 30*time.Second, "Update rate.")
	flag.StringVar(&connectivityEndpoint, "connectivity-endpoint", "8.8.8.8:53", "Server that accepts tcp connections.")
	flag.StringVar(&ipEndpoint, "ip-endpoint", "https://api.ipify.org", "Server returns your public ip address over http.")
	flag.StringVar(&geoEndpoint, "geo-endpoint", "http://ip-api.com/json/", "IP geolocation endpoint")
	flag.DurationVar(&notificationExpiry, "notification-expiration", 10*time.Second, "Notifications expiry duration.")
	flag.Parse()

	state := InitState()

	state.QueryConnectivity()
	state.QueryPublicIP()
	state.QueryGeoInfo()

	dbusConnSession, err := dbus.SessionBus()
	if err != nil {
		log.Fatal(err)
	}

	notifier := notify.New(dbusConnSession, "Internet Notify Agent")

	notifyState(state, notifier)

	if initialOnly {
		return
	}

	wasConnected := state.Connected
	oldPublicIP := state.PublicIP

	for range time.Tick(tick) {
		state.QueryConnectivity()
		state.QueryPublicIP()

		changedConnectivity := wasConnected != state.Connected
		changedPublicIP := state.PublicIP != "" && oldPublicIP != state.PublicIP
		reconnected := changedConnectivity && state.Connected

		if changedPublicIP || reconnected {
			state.QueryGeoInfo()
		}

		if changedConnectivity || changedPublicIP {
			notifyState(state, notifier)
		}

		wasConnected = state.Connected
		if state.PublicIP != "" {
			oldPublicIP = state.PublicIP
		}
	}
}

func notifyState(state *State, notifier *notify.Notifier) {
	var msg string
	if state.Connected {
		msg = "Connected"
		if state.PublicIP != "" {
			msg += " " + state.PublicIP
		}
		if state.Geo != nil {
			msg += " " + state.Geo.Country
		}
	} else {
		msg = "Disconnected"
	}
	if err := notifier.Normal("Internet", msg, int32(notificationExpiry / time.Millisecond)); err != nil {
		log.Printf("Failed to send notification: %v", err)
	}
}
