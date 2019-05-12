// Package he implements the hosting environment to land peer scripts for HBI
package he

// Reactor is the interface optionally implemented by a type whose instances are
// to be exposed to a `HostingEnv` by calling `he.ExposeReactor()`.
//
// If a reactor object does not implement this interface, all its exported
// fields and methods will be exposed.
type Reactor interface {
	NamesToExpose() []string
}
