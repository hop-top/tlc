package core

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// RecipeInfo is a recipe's header as seen by List: enough to pick one
// without parsing every file fully.
type RecipeInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"` // directory or "builtin"
	Path        string `json:"path"`
}

// RecipeNotFoundError reports a reference no layer could resolve.
type RecipeNotFoundError struct {
	Ref  string
	Dirs []string
}

func (e *RecipeNotFoundError) Error() string {
	if len(e.Dirs) == 0 {
		return fmt.Sprintf("recipe %q not found (no recipe directories configured)", e.Ref)
	}
	return fmt.Sprintf("recipe %q not found in %s", e.Ref, strings.Join(e.Dirs, ", "))
}

// IsRecipeNotFound reports whether err wraps a RecipeNotFoundError.
func IsRecipeNotFound(err error) bool {
	var nf *RecipeNotFoundError
	return asRecipeNotFound(err, &nf)
}

func asRecipeNotFound(err error, target **RecipeNotFoundError) bool {
	return errors.As(err, target)
}

// BuiltinSource is the Source of recipes served from the embedded set.
const BuiltinSource = "builtin"

// DirLocator resolves recipes from an ordered list of directories, then
// from an optional embedded set. An unpinned name takes the first
// directory that has it (highest version there); a pinned name@version is
// searched across every layer in order. A path is opened directly.
type DirLocator struct {
	Dirs    []string
	Builtin fs.FS
}

// Locate resolves ref: a file path, a name, or name@version.
func (l *DirLocator) Locate(ref string) (*Recipe, error) {
	if isRecipePath(ref) {
		return loadRecipeFile(ref, filepath.Dir(ref))
	}
	name, version, _ := strings.Cut(ref, "@")
	if !recipeNameRe.MatchString(name) {
		return nil, fmt.Errorf("invalid recipe reference %q: want a name, name@version or a file path", ref)
	}
	for _, dir := range l.Dirs {
		if info, ok := pickRecipe(scanRecipeDir(dir), name, version); ok {
			return loadRecipeFile(info.Path, dir)
		}
	}
	if l.Builtin != nil {
		if info, ok := pickRecipe(scanRecipeFS(l.Builtin), name, version); ok {
			return loadRecipeFS(l.Builtin, info.Path)
		}
	}
	dirs := append([]string(nil), l.Dirs...)
	if l.Builtin != nil {
		dirs = append(dirs, BuiltinSource)
	}
	return nil, &RecipeNotFoundError{Ref: ref, Dirs: dirs}
}

// List returns every recipe header, in layer order, then by name and
// descending version within a layer. Missing directories are skipped.
func (l *DirLocator) List() ([]RecipeInfo, error) {
	var out []RecipeInfo
	for _, dir := range l.Dirs {
		out = append(out, scanRecipeDir(dir)...)
	}
	if l.Builtin != nil {
		out = append(out, scanRecipeFS(l.Builtin)...)
	}
	return out, nil
}

// isRecipePath treats anything with a separator, a recipe extension, or
// an existing file behind it as a path rather than a name.
func isRecipePath(ref string) bool {
	if strings.ContainsAny(ref, `/\`) {
		return true
	}
	switch strings.ToLower(filepath.Ext(ref)) {
	case extYAML, extYML, extJSON:
		return true
	}
	_, err := os.Stat(ref)
	return err == nil
}

func loadRecipeFile(path, source string) (*Recipe, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open recipe: %w", err)
	}
	defer f.Close()
	r, err := ParseRecipe(f, path)
	if err != nil {
		return nil, err
	}
	r.Source = source
	return r, nil
}

func loadRecipeFS(fsys fs.FS, path string) (*Recipe, error) {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("read builtin recipe: %w", err)
	}
	r, err := ParseRecipe(bytes.NewReader(data), BuiltinSource+":"+path)
	if err != nil {
		return nil, err
	}
	r.Source = BuiltinSource
	return r, nil
}

// pickRecipe chooses the highest version of name, or the exact version
// when pinned, from one layer's headers.
func pickRecipe(infos []RecipeInfo, name, version string) (RecipeInfo, bool) {
	var best RecipeInfo
	found := false
	for _, info := range infos {
		if info.Name != name {
			continue
		}
		if version != "" {
			if info.Version == version {
				return info, true
			}
			continue
		}
		if !found || CompareRecipeVersions(info.Version, best.Version) > 0 {
			best, found = info, true
		}
	}
	return best, found
}

// recipeHeader is the lenient header decode used for scanning.
type recipeHeader struct {
	Name        string `yaml:"recipe"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

func scanRecipeDir(dir string) []RecipeInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var infos []RecipeInfo
	for _, e := range entries {
		if e.IsDir() || !isRecipeExt(e.Name()) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if info, ok := headerInfo(data, dir, path); ok {
			infos = append(infos, info)
		}
	}
	sortRecipeInfos(infos)
	return infos
}

func scanRecipeFS(fsys fs.FS) []RecipeInfo {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil
	}
	var infos []RecipeInfo
	for _, e := range entries {
		if e.IsDir() || !isRecipeExt(e.Name()) {
			continue
		}
		data, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			continue
		}
		if info, ok := headerInfo(data, BuiltinSource, e.Name()); ok {
			infos = append(infos, info)
		}
	}
	sortRecipeInfos(infos)
	return infos
}

// Recipe file extensions; directory scans only pick up the YAML ones.
const (
	extYAML = ".yaml"
	extYML  = ".yml"
	extJSON = ".json"
)

func isRecipeExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == extYAML || ext == extYML
}

func headerInfo(data []byte, source, path string) (RecipeInfo, bool) {
	var h recipeHeader
	if err := yaml.Unmarshal(data, &h); err != nil || h.Name == "" {
		return RecipeInfo{}, false
	}
	return RecipeInfo{Name: h.Name, Version: h.Version, Description: h.Description, Source: source, Path: path}, true
}

func sortRecipeInfos(infos []RecipeInfo) {
	sort.SliceStable(infos, func(i, j int) bool {
		if infos[i].Name != infos[j].Name {
			return infos[i].Name < infos[j].Name
		}
		return CompareRecipeVersions(infos[i].Version, infos[j].Version) > 0
	})
}

// CompareRecipeVersions orders two versions semver-style without a
// dependency: numeric core parts (missing parts are zero, so 0.1 == 0.1.0),
// a release above any of its pre-releases, and pre-release parts compared
// numerically when both are numbers.
func CompareRecipeVersions(a, b string) int {
	coreA, preA, _ := strings.Cut(strings.TrimPrefix(a, "v"), "-")
	coreB, preB, _ := strings.Cut(strings.TrimPrefix(b, "v"), "-")
	if c := compareVersionCore(coreA, coreB); c != 0 {
		return c
	}
	switch {
	case preA == "" && preB == "":
		return 0
	case preA == "":
		return 1
	case preB == "":
		return -1
	}
	return compareVersionParts(strings.Split(preA, "."), strings.Split(preB, "."))
}

func compareVersionCore(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for len(pa) < len(pb) {
		pa = append(pa, "0")
	}
	for len(pb) < len(pa) {
		pb = append(pb, "0")
	}
	return compareVersionParts(pa, pb)
}

func compareVersionParts(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if c := compareVersionPart(a[i], b[i]); c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

func compareVersionPart(a, b string) int {
	na, errA := strconv.Atoi(a)
	nb, errB := strconv.Atoi(b)
	switch {
	case errA == nil && errB == nil:
		return cmpInt(na, nb)
	case errA == nil:
		return -1 // numeric identifiers sort before alphanumeric ones
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
