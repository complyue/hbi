// Package proto defines the building blocks for API defined protocols with HBI
package proto

// InitMagicFunction is the prototype for magic functions to be called on HBI wire connection made.
// Such a function should be named `__hbi_init__` on the `HostingCtx`.
type InitMagicFunction = func(po PostingEnd, ho HostingEnd)

// CleanupMagicFunction is the prototype for magic functions to be called on HBI wire disconnected.
// Such a function should be named `__hbi_cleanup__` on the `HostingCtx`
type CleanupMagicFunction = func(err error)
