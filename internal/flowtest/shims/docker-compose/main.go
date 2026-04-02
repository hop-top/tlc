// Package main is the docker-compose shim for the flowtest sandbox.
package main

import "hop.top/tlc/internal/flowtest/shims/shims"

func main() { shims.Run("docker-compose") }
