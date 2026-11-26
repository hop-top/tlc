package core

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hop.top/flo/spec"
)

// DirLocator resolves recipes from an ordered list of directories, then
// from an optional embedded set. An unpinned name takes the first
// directory that has it (highest version there); a pinned name@version is
// searched across every layer in order. A path is opened directly.
//
// Each layer is read through spec's locator with the recipe vocabulary
// shimmed to flow vocabulary; results are re-pointed at the real files.
type DirLocator struct {
	Dirs    []string
	Builtin fs.FS
}

const builtinPrefix = spec.BuiltinSource + ":"

// Locate resolves ref: a file path, a name, or name@version.
func (l *DirLocator) Locate(ref string) (*Recipe, error) {
	if isRecipePath(ref) {
		return loadRecipeFile(ref, filepath.Dir(ref))
	}
	for _, dir := range l.Dirs {
		layer := spec.DirLocator{Builtin: shimFS{os.DirFS(dir)}}
		rec, err := layer.Locate(ref)
		if err == nil {
			rec.Source = dir
			name := strings.TrimPrefix(rec.Path, builtinPrefix)
			rec.Path = filepath.Join(dir, name)
			if raw, rerr := os.ReadFile(rec.Path); rerr == nil {
				rec.Hash = rawHash(raw)
			}
			return rec, nil
		}
		if !spec.IsFlowNotFound(err) {
			return nil, recipeWording(repointLayerError(err, dir))
		}
	}
	if l.Builtin != nil {
		layer := spec.DirLocator{Builtin: shimFS{l.Builtin}}
		rec, err := layer.Locate(ref)
		if err == nil {
			if raw, rerr := fs.ReadFile(l.Builtin, strings.TrimPrefix(rec.Path, builtinPrefix)); rerr == nil {
				rec.Hash = rawHash(raw)
			}
			return rec, nil
		}
		if !spec.IsFlowNotFound(err) {
			return nil, recipeWording(err)
		}
	}
	// No layer has it: let spec reject a malformed ref, else report every
	// layer searched.
	dirs := append([]string(nil), l.Dirs...)
	if l.Builtin != nil {
		dirs = append(dirs, spec.BuiltinSource)
	}
	_, err := (&spec.DirLocator{Dirs: nil}).Locate(ref)
	if err != nil && !spec.IsFlowNotFound(err) {
		return nil, recipeWording(err)
	}
	return nil, recipeWording(&RecipeNotFoundError{Ref: ref, Dirs: dirs})
}

// List returns every recipe header, in layer order, then by name and
// descending version within a layer. Missing directories are skipped.
func (l *DirLocator) List() ([]RecipeInfo, error) {
	var out []RecipeInfo
	for _, dir := range l.Dirs {
		infos, err := (&spec.DirLocator{Builtin: shimFS{os.DirFS(dir)}}).List()
		if err != nil {
			return nil, recipeWording(err)
		}
		for _, info := range infos {
			info.Source = dir
			info.Path = filepath.Join(dir, info.Path)
			out = append(out, info)
		}
	}
	if l.Builtin != nil {
		infos, err := (&spec.DirLocator{Builtin: shimFS{l.Builtin}}).List()
		if err != nil {
			return nil, recipeWording(err)
		}
		out = append(out, infos...)
	}
	return out, nil
}

// repointLayerError swaps the synthetic "builtin:<file>" a layer reports
// for the real path.
func repointLayerError(err error, dir string) error {
	msg := err.Error()
	if !strings.HasPrefix(msg, builtinPrefix) {
		return err
	}
	rest := msg[len(builtinPrefix):]
	name, tail, ok := strings.Cut(rest, ": ")
	if !ok {
		return err
	}
	return &wordedError{msg: filepath.Join(dir, name) + ": " + tail, err: err}
}

// isRecipePath treats anything with a separator, a recipe extension, or an
// existing file behind it as a path rather than a name.
func isRecipePath(ref string) bool {
	if strings.ContainsAny(ref, `/\`) {
		return true
	}
	switch strings.ToLower(filepath.Ext(ref)) {
	case ".yaml", ".yml", extJSON:
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
	rec, err := ParseRecipe(f, path)
	if err != nil {
		return nil, err
	}
	rec.Source = source
	return rec, nil
}

var recipeRefName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// LocateNamed resolves ref as a name or name@version through the search
// path only: no file path is accepted, and the working directory is never
// consulted (Locate and spec's own locator both treat a bare name that
// exists as a file as a path). It is the lookup for callers that take the
// reference from an untrusted request.
func (l *DirLocator) LocateNamed(ref string) (*Recipe, error) {
	name, version, _ := strings.Cut(ref, "@")
	if !recipeRefName.MatchString(name) {
		return nil, fmt.Errorf("invalid recipe reference %q: want a recipe name or name@version, not a path", ref)
	}
	for _, dir := range l.Dirs {
		infos, err := (&spec.DirLocator{Builtin: shimFS{os.DirFS(dir)}}).List()
		if err != nil {
			return nil, recipeWording(repointLayerError(err, dir))
		}
		if info, ok := pickRecipeInfo(infos, name, version); ok {
			return loadRecipeFile(filepath.Join(dir, info.Path), dir)
		}
	}
	dirs := append([]string(nil), l.Dirs...)
	if l.Builtin != nil {
		rec, err := l.locateBuiltin(name, version)
		if err != nil || rec != nil {
			return rec, err
		}
		dirs = append(dirs, spec.BuiltinSource)
	}
	return nil, recipeWording(&RecipeNotFoundError{Ref: ref, Dirs: dirs})
}

// locateBuiltin looks name up in the embedded recipes; a nil recipe with a
// nil error means it is not there.
func (l *DirLocator) locateBuiltin(name, version string) (*Recipe, error) {
	infos, err := (&spec.DirLocator{Builtin: shimFS{l.Builtin}}).List()
	if err != nil {
		return nil, recipeWording(err)
	}
	info, ok := pickRecipeInfo(infos, name, version)
	if !ok {
		return nil, nil
	}
	data, err := fs.ReadFile(l.Builtin, info.Path)
	if err != nil {
		return nil, fmt.Errorf("read builtin recipe: %w", err)
	}
	rec, err := ParseRecipe(bytes.NewReader(data), builtinPrefix+info.Path)
	if err != nil {
		return nil, err
	}
	rec.Source = spec.BuiltinSource
	return rec, nil
}

// pickRecipeInfo takes the first header named name: the one at version
// when pinned, else the highest (List orders versions descending).
func pickRecipeInfo(infos []RecipeInfo, name, version string) (RecipeInfo, bool) {
	for _, info := range infos {
		if info.Name == name && (version == "" || info.Version == version) {
			return info, true
		}
	}
	return RecipeInfo{}, false
}
