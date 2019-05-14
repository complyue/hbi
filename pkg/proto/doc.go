// Package proto defines the building blocks for API defined protocols with HBI
package proto

// InitMagicFunction is the prototype for magic functions to be called on HBI
// wire connection made.
//
// Such a function should be exposed with name `__hbi_init__` from the `HostingEnv`.
//
// note: this interface should really be defined in `he` package, but that'll
// create cyclic imports between `proto` and `he`.
type InitMagicFunction = func(po *PostingEnd, ho *HostingEnd)

// CleanupMagicFunction is the prototype for magic functions to be called on HBI
// wire disconnected.
//
// Such a function should be exposed with name `__hbi_cleanup__` from the `HostingEnv`.
//
// note: this interface should really be defined in `he` package, but that'll
// create cyclic imports between `proto` and `he`.
type CleanupMagicFunction = func(discReason string)
