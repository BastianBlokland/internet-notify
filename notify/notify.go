// Source: github.com/omeid/upower-notify/notify/notify.go
// A minimal binding for DBus Desktop Notifications,
// it is designed to be as simple as send-notify.

package notify

import (
	"github.com/godbus/dbus/v5"
)

type Notifier struct {
	dbus dbus.BusObject

	app string
}

func New(conn *dbus.Conn, app string) *Notifier {
	notification := conn.Object("org.freedesktop.Notifications", "/org/freedesktop/Notifications")
	return &Notifier{dbus: notification, app: app}
}

func (n *Notifier) Normal(summary string, body string, expireTimeout int32) error {
	return n.dbus.Call("org.freedesktop.Notifications.Notify", 0,
		n.app,      // app_name
		uint32(0),  // replaces_id
		"",         // app_icon
		summary,    // summary
		body,       // body
		[]string{}, // actions
		map[string]dbus.Variant{"urgency": dbus.MakeVariant(byte(1))}, // hints (urgency: normal)
		expireTimeout, // expire_timeout
	).Err
}
