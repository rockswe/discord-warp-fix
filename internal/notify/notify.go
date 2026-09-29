// Package notify shows a desktop notification. Failures are ignored:
// notifications are a courtesy, never required.
package notify

// Title is shown on every notification.
const Title = "Discord WARP Fix"

// Send shows msg.
func Send(msg string) { _ = send(msg) }
