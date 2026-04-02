// Package main is the npm shim for the flowtest sandbox.
// Cassette-backed: records/replays npm interactions without network access.
package main

import "hop.top/tlc/internal/flowtest/shims/shimlib"

func main() { shimlib.Run("npm") }
