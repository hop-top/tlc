// Package main is the uv shim for the flowtest sandbox.
// Cassette-backed: records/replays uv interactions without network access.
package main

import "hop.top/tlc/internal/flowtest/shims/shimlib"

func main() { shimlib.Run("uv") }
