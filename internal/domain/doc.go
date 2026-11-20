// Package domain holds the core business model of the notification system:
// notifications and their lifecycle, per-channel delivery rules, batches,
// delivery attempts and message templates.
//
// The package is deliberately free of infrastructure concerns (HTTP, SQL,
// queues). Every other layer depends on it; it depends on nothing but the
// standard library and a UUID type.
package domain
