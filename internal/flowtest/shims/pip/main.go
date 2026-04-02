// Package main is the pip shim for the flowtest sandbox.
// Cassette-backed: records/replays pip interactions without network access.
package main

import "hop.top/tlc/internal/flowtest/shims/shimlib"

func main() { shimlib.Run("pip") }
