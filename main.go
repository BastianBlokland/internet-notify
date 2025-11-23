package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"internet-notify/notify"
)

var (
	initialOnly          bool
	tick                 time.Duration
	connectivityEndpoint string
	ipEndpoint           string
	geoEndpoint          string
	notificationExpiry   time.Duration

	notificationExpiryMilliseconds int32
)

type GeoInfo struct {
	Status  string `json:"status"`
	Country string `json:"country"`
}

type State struct {
	HttpClient http.Client
	Connected  bool
	PublicIp   string
	Geo        *GeoInfo
}

func InitState() (*State, error) {
	client := http.Client{Timeout: 5 * time.Second}
	return &State{HttpClient: client}, nil
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

func (s *State) QueryPublicIp() {
	if !s.Connected {
		s.PublicIp = ""
		return
	}
	resp, err := s.HttpClient.Get(ipEndpoint)
	if err != nil {
		s.PublicIp = ""
		return
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		s.PublicIp = ""
		return
	}
	s.PublicIp = strings.TrimSpace(string(ip))
}

func (s *State) QueryGeoInfo() {
	if !s.Connected || len(s.PublicIp) == 0 {
		s.Geo = nil
		return
	}
	url := geoEndpoint + s.PublicIp
	resp, err := s.HttpClient.Get(url)
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

	notificationExpiryMilliseconds = int32(notificationExpiry / time.Millisecond)

	state, err := InitState()
	if err != nil {
		log.Fatal(err)
	}

	state.QueryConnectivity()
	state.QueryPublicIp()
	state.QueryGeoInfo()

	notifier, err := notify.New("Internet Notify Agent")
	if err != nil {
		log.Fatal(err)
	}

	notifyState(state, notifier)

	if initialOnly {
		return
	}

	var wasConnected = state.Connected
	var oldPublicIp = state.PublicIp

	for range time.Tick(tick) {
		state.QueryConnectivity()
		state.QueryPublicIp()

		changedConnectivity := wasConnected != state.Connected
		changedPublicIp := (len(state.PublicIp) > 0) && (oldPublicIp != state.PublicIp)

		if changedPublicIp {
			state.QueryGeoInfo()
		}

		if changedConnectivity || changedPublicIp {
			notifyState(state, notifier)
		}

		wasConnected = state.Connected
		oldPublicIp = state.PublicIp
	}
}

func notifyState(state *State, notifier *notify.Notifier) {
	if state.Connected {
		msg := "Connected"
		if len(state.PublicIp) > 0 {
			msg += fmt.Sprintf(" %s", state.PublicIp)
		}
		if state.Geo != nil {
			msg += fmt.Sprintf(" %s", state.Geo.Country)
		}
		notifier.Normal("Internet", msg, notificationExpiryMilliseconds)
	} else {
		notifier.Normal("Internet", "Disconnected", notificationExpiryMilliseconds)
	}
}
