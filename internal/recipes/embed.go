// Package recipes holds the recipes compiled into tlc. They form the
// lowest layer of the recipe search path: available in every project
// with nothing to copy, and shadowed by any same-name recipe in
// recipe.dir, the project's recipes dir or the user's.
//
// Each recipe's test fixtures sit beside it under fixtures/<recipe>/ so
// `tlc recipe test` on the source file finds them; only the top-level
// YAML files are embedded.
package recipes

import (
	"embed"
	"io/fs"
)

//go:embed *.yaml
var files embed.FS

// FS returns the built-in recipe set, one recipe per top-level file.
func FS() fs.FS { return files }
