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

const (
	networkdService      = "org.freedesktop.network1"
	networkdLinkIface    = "org.freedesktop.network1.Link"
	dbusPropsIface       = "org.freedesktop.DBus.Properties"
	dbusPropsChanged     = dbusPropsIface + ".PropertiesChanged"
	dbusNameOwnerChanged = "org.freedesktop.DBus.NameOwnerChanged"

	minCheckInterval = 10 * time.Second
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

// shouldTriggerCheck returns true if the signal indicates a state transition
// worth checking connectivity for. Only reacts to definitively connected
// (routable) or disconnected (no-carrier, off, missing) states, ignoring
// intermediate states like carrier and dormant to avoid consuming the
// rate-limit window before the routable signal arrives.
func shouldTriggerCheck(sig *dbus.Signal) bool {
	if sig.Name != dbusPropsChanged {
		return false
	}
	if len(sig.Body) < 2 {
		return false
	}
	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return false
	}
	v, ok := changed["OperationalState"]
	if !ok {
		return false
	}
	state, ok := v.Value().(string)
	if !ok {
		return false
	}
	switch state {
	case "routable", "no-carrier", "off", "missing":
		return true
	default:
		return false
	}
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
		log.Fatalf("Failed to connect to session bus: %v", err)
	}

	notifier := notify.New(dbusConnSession, "Internet Notify Agent")

	notifyState(state, notifier)

	if initialOnly {
		return
	}

	// Attempt to subscribe to systemd-networkd signals for immediate checks.
	var signals <-chan *dbus.Signal
	dbusConnSystem, err := dbus.SystemBus()
	if err != nil {
		log.Printf("Failed to connect to system bus, networkd signals disabled: %v", err)
	} else {
		err1 := dbusConnSystem.AddMatchSignal(
			dbus.WithMatchSender(networkdService),
			dbus.WithMatchInterface(dbusPropsIface),
			dbus.WithMatchMember("PropertiesChanged"),
			dbus.WithMatchArg(0, networkdLinkIface),
		)
		err2 := dbusConnSystem.AddMatchSignal(
			dbus.WithMatchInterface("org.freedesktop.DBus"),
			dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, networkdService),
		)
		if err1 != nil {
			log.Printf("Failed to subscribe to networkd signals, falling back to polling: %v", err1)
		}
		if err2 != nil {
			log.Printf("Failed to subscribe to networkd signals, falling back to polling: %v", err2)
		}
		if err1 == nil && err2 == nil {
			ch := make(chan *dbus.Signal, 16)
			dbusConnSystem.Signal(ch)
			signals = ch
		}
	}

	wasConnected := state.Connected
	oldPublicIP := state.PublicIP
	lastCheck := time.Now()

	ticker := time.NewTicker(tick)
	for {
		select {
		case sig := <-signals:
			if sig.Name == dbusNameOwnerChanged {
				log.Printf("systemd-networkd service changed")
				continue
			}
			if !shouldTriggerCheck(sig) {
				continue
			}
			if time.Since(lastCheck) < minCheckInterval {
				continue
			}
		case <-ticker.C:
			if time.Since(lastCheck) < minCheckInterval {
				continue
			}
		}
		checkAndNotify(state, notifier, &wasConnected, &oldPublicIP)
		lastCheck = time.Now()
	}
}

func checkAndNotify(state *State, notifier *notify.Notifier, wasConnected *bool, oldPublicIP *string) {
	state.QueryConnectivity()
	state.QueryPublicIP()

	changedConnectivity := *wasConnected != state.Connected
	changedPublicIP := state.PublicIP != "" && *oldPublicIP != state.PublicIP

	if changedPublicIP {
		state.QueryGeoInfo()
	}
	if changedConnectivity || changedPublicIP {
		notifyState(state, notifier)
	}

	*wasConnected = state.Connected
	if state.PublicIP != "" {
		*oldPublicIP = state.PublicIP
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
	if err := notifier.Normal("Internet", msg, int32(notificationExpiry/time.Millisecond)); err != nil {
		log.Printf("Failed to send notification: %v", err)
	}
}
