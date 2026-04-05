// Package hdl registers custom URL schemes with the host operating system
// so that clicking a scheme://... link opens the correct application.
//
// Usage:
//
//	err := hdl.Register("ctxt", "/Applications/ctxt.app")
//
// On platforms where runtime registration is impossible (iOS, sandboxed apps),
// use the [generate] subpackage to emit static config snippets instead.
package hdl
