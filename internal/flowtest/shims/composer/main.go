// Package main is the composer shim for the flowtest sandbox.
// Cassette-backed: records/replays composer interactions without network access.
package main

import "hop.top/tlc/internal/flowtest/shims/shimlib"

func main() { shimlib.Run("composer") }
