package main

import (
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
	notificationExpiry   time.Duration

	notificationExpiryMilliseconds int32
)

type State struct {
	httpClient http.Client
	connected  bool
	publicIp   string
}

func InitState() (*State, error) {
	client := http.Client{Timeout: 5 * time.Second}
	return &State{httpClient: client}, nil
}

func (s *State) QueryConnectivity() {
	timeout := 5 * time.Second
	conn, err := net.DialTimeout("tcp", connectivityEndpoint, timeout)
	if err != nil {
		s.connected = false
	} else {
		s.connected = true
		conn.Close()
	}
}

func (s *State) QueryPublicIp() {
	if !s.connected {
		s.publicIp = ""
		return
	}
	resp, err := s.httpClient.Get(ipEndpoint)
	if err != nil {
		s.publicIp = ""
		return
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		s.publicIp = ""
		return
	}
	s.publicIp = strings.TrimSpace(string(ip))
}

func main() {

	flag.BoolVar(&initialOnly, "initialOnly", false, "Exit after sending the initial notification.")
	flag.DurationVar(&tick, "tick", 60*time.Second, "Update rate.")
	flag.StringVar(&connectivityEndpoint, "connectivity-endpoint", "8.8.8.8:53", "Server that accepts tcp connections.")
	flag.StringVar(&ipEndpoint, "ip-endpoint", "https://api.ipify.org", "Server returns your public ip address over http.")
	flag.DurationVar(&notificationExpiry, "notification-expiration", 10*time.Second, "Notifications expiry duration.")
	flag.Parse()

	notificationExpiryMilliseconds = int32(notificationExpiry / time.Millisecond)

	state, err := InitState()
	if err != nil {
		log.Fatal(err)
	}

	state.QueryConnectivity()
	state.QueryPublicIp()

	notifier, err := notify.New("Internet Notify Agent")
	if err != nil {
		log.Fatal(err)
	}

	notifyState(state, notifier)

	if initialOnly {
		return
	}

	var wasConnected = state.connected
	var oldPublicIp = state.publicIp

	for range time.Tick(tick) {
		state.QueryConnectivity()
		state.QueryPublicIp()

		changedConnectivity := wasConnected != state.connected
		changedPublicIp := (len(state.publicIp) > 0) && (oldPublicIp != state.publicIp)

		if changedConnectivity || changedPublicIp {
			notifyState(state, notifier)
		}

		wasConnected = state.connected
		oldPublicIp = state.publicIp
	}
}

func notifyState(state *State, notifier *notify.Notifier) {
	if state.connected {
		msg := "Connected"
		if len(state.publicIp) > 0 {
			msg += fmt.Sprintf(" (%s)", state.publicIp)
		}
		notifier.Normal("Internet", msg, notificationExpiryMilliseconds)

	} else {
		notifier.Normal("Internet", "Disconnected", notificationExpiryMilliseconds)
	}
}
