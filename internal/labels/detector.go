package labels

import (
	"os"
	"path/filepath"
)

// DetectProjectType attempts to detect the project type based on files in the path.
//
// Detection reaches every type GetTemplates has a case for, which is the
// property that keeps the two in step: a type worth offering under
// --type is a type worth guessing, and a type nothing can guess is a
// type a user only finds by reading the source.
//
// Detection is a GUESS, and every branch is one a user can override with
// --type. That is what lets the branches below use a single marker file
// where a stricter rule would need several: guessing monorepo from
// `go.work` and being wrong costs one flag, while failing to guess it
// costs a user who never learns the template exists.
//
// ORDER IS THE WHOLE DESIGN HERE. Several shapes share a marker file, so
// the branches run most-specific-first and each comment below says which
// broader branch it is standing in front of and why. Read top to bottom.
func DetectProjectType(path string) ProjectType {
	// Monorepo first, ahead of EVERY language branch.
	//
	// A workspace root carries the same markers its members do — a
	// `go.work` root almost always has a root `go.mod`, a pnpm workspace
	// root has a root `package.json` — so any language branch placed
	// above this one would claim the root and the workspace file would
	// never be read. The workspace file is also the more specific
	// signal: `go.mod` says "Go lives here", `go.work` says "several
	// modules live here and this is the thing that composes them", which
	// is the fact the label set is about.
	//
	// The three markers are the declaration files of the three workspace
	// tools, not heuristics: none of them appears in a single-package
	// repo, so this branch cannot capture one. A Go binary with a plain
	// `go.mod` and no `go.work` falls straight through to the go.mod
	// branch below.
	for _, marker := range []string{"pnpm-workspace.yaml", "go.work", "turbo.json"} {
		if exists(filepath.Join(path, marker)) {
			return TypeMonorepo
		}
	}

	// Infrastructure before the language branches, because an infra repo
	// is frequently ALSO a Go or Node repo — Terraform providers,
	// Pulumi programs and CDK apps all carry a language manifest — and
	// in those the infrastructure is the product while the language is
	// an implementation detail.
	//
	// It is below monorepo because an infra directory inside a
	// workspace is a member, not the root, and the root is what gets
	// labelled.
	if isInfra(path) {
		return TypeInfra
	}

	if exists(filepath.Join(path, "go.mod")) {
		// A Go module with no `cmd/` and no root `main.go` is a library.
		//
		// This is the `cmd/` test the removed go-socket branch was
		// reaching for, finally load-bearing. It is stated as the
		// ABSENCE of both entrypoint spellings rather than the presence
		// of one, because those two are the only places a Go binary's
		// entrypoint can live: `main.go` at the root for a single-binary
		// repo, `cmd/<name>/main.go` for one or more named binaries.
		//
		// Absence is also the safe direction for the contention the
		// go-binary fixtures pin. `go.mod` + `main.go` and `go.mod` +
		// `cmd/tool/main.go` both have an entrypoint, so both stay
		// go-binary; only a module with neither — which is what a
		// library IS — reaches TypeLibrary.
		if !hasGoEntrypoint(path) {
			return TypeLibrary
		}
		return TypeGoBinary
	}

	if exists(filepath.Join(path, "package.json")) {
		// React before node-backend: `src/App.tsx` is the specific
		// signal and `package.json` is the general one, so testing the
		// specific first is what keeps a React app from being read as a
		// service. Inverting these two is the single most likely way to
		// regress this function, which is why the React fixture is
		// pinned in two separate tests.
		if exists(filepath.Join(path, "src", "App.tsx")) || exists(filepath.Join(path, "src", "App.jsx")) {
			return TypeReactFrontend
		}
		// Everything else with a `package.json` is a Node backend.
		//
		// This branch is deliberately a catch-all rather than a search
		// for a server marker. There is no equivalent of `src/App.tsx`
		// on the server side — an entrypoint may be `index.js`,
		// `server.ts`, `src/main.ts` or a `bin` entry, and the framework
		// may be Express, Fastify, Nest or none — so any marker list
		// would be a list of the frameworks that existed when it was
		// written, and a miss would silently fall through to generic.
		//
		// A Node package that is really a LIBRARY does land here rather
		// than on TypeLibrary, and that is a known limit: `package.json`
		// alone cannot tell them apart, since a library and a service
		// declare the same file. `--type library` is the answer for
		// that repo. The Go side can distinguish them only because Go
		// puts entrypoints in two known places.
		return TypeNodeBackend
	}

	if exists(filepath.Join(path, "manage.py")) || exists(filepath.Join(path, "requirements.txt")) {
		return TypePythonMVC
	}

	return TypeGeneric
}

// hasGoEntrypoint reports whether a Go module builds a binary, by
// looking in the only two places `package main` can live in a
// conventional layout: a root `main.go`, or a `cmd/` tree.
//
// `cmd/` is tested for existence rather than walked for a `main.go`. A
// `cmd/` directory in a Go repo has one meaning, and a repo that has
// created one is declaring an intent to ship a binary even on the commit
// where it is still empty.
func hasGoEntrypoint(path string) bool {
	return exists(filepath.Join(path, "main.go")) || exists(filepath.Join(path, "cmd"))
}

// isInfra reports whether the directory declares infrastructure.
//
// `helm/` and `k8s/` are directory conventions and are tested directly.
// Terraform has no such directory — `.tf` files sit at the root of a
// module — so that one needs a glob, which is why this is a function
// rather than another entry in a marker list.
func isInfra(path string) bool {
	for _, dir := range []string{"terraform", "helm", "k8s", "kubernetes"} {
		if exists(filepath.Join(path, dir)) {
			return true
		}
	}
	// A `.tf` at the root is the Terraform module convention. The error
	// is ignored on purpose: filepath.Glob fails only on a malformed
	// pattern, and this one is a literal.
	matches, _ := filepath.Glob(filepath.Join(path, "*.tf")) //nolint:errcheck // ErrBadPattern only; the pattern is a literal
	return len(matches) > 0
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}
