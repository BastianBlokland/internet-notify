// Source: github.com/omeid/upower-notify/notify/notify.go
// A minimal binding for DBus Desktop Notifications,
// it is designed to be as simple as send-notify.

package notify

import (
	"github.com/godbus/dbus/v5"
)

type Urgency byte

const (
	Low Urgency = iota
	Normal
	Critical
)

type Notifier struct {
	dbus dbus.BusObject

	app string
}

func New(app string) (*Notifier, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return nil, err
	}
	notification := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	return &Notifier{dbus: notification, app: app}, nil
}

func (n *Notifier) Low(summary string, body string, expireTimeout int32) error {
	return n.Send(summary, body, Low, expireTimeout)
}

func (n *Notifier) Normal(summary string, body string, expireTimeout int32) error {
	return n.Send(summary, body, Normal, expireTimeout)
}

func (n *Notifier) Critical(summary string, body string, expireTimeout int32) error {
	return n.Send(summary, body, Critical, expireTimeout)
}

func (n *Notifier) Send(summary string, body string, urgency Urgency, expireTimeout int32) error {
	return n.dbus.Call("org.freedesktop.Notifications.Notify", 0,
		n.app, // app_name
		uint32(0), // replaces_id
		"", // app_icon
		summary, // summary
		body, // body
		[]string{}, // actions
		map[string]dbus.Variant{"urgency": dbus.MakeVariant(urgency)},
		expireTimeout,
	).Err
}
