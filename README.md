# Internet-Notify

A lightweight tool that sends desktop notifications when your internet connectivity changes or your public IP address changes.

Inspired by [upower-notify](https://github.com/omeid/upower-notify).

## How it works

On startup, internet-notify checks connectivity and sends an initial notification. It then monitors for changes using two mechanisms:

- **systemd-networkd signals** (via D-Bus): reacts immediately when a network interface becomes routable (e.g. after connecting to Wi-Fi) or loses connectivity. Requires systemd-networkd; falls back to polling-only if unavailable.
- **Periodic polling** (default every 30s): fallback to catch any changes not covered by signals.

Checks are rate-limited to at most once every 10 seconds to avoid hammering the network during rapid interface state changes.

## Usage

Launch `internet-notify` when starting your desktop environment.

```
internet-notify [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-tick` | `30s` | Polling interval (fallback) |
| `-connectivity-endpoint` | `8.8.8.8:53` | TCP endpoint used to test connectivity |
| `-ip-endpoint` | `https://api.ipify.org` | HTTP endpoint that returns your public IP |
| `-geo-endpoint` | `http://ip-api.com/json/` | IP geolocation endpoint |
| `-notification-expiration` | `10s` | How long notifications stay on screen |
| `-initialOnly` | `false` | Send one notification on startup then exit |

## Credits

`notify/notify.go` is based on [`github.com/omeid/upower-notify/notify/notify.go`](https://github.com/omeid/upower-notify).
