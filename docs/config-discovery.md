# TLC Config Discovery: Custom Walk-Up Implementation

## Overview

TLC uses a **custom directory traversal** (`findAllConfigsForMode()`) to discover
project configuration files, rather than relying on Viper's static path search.
The walk-up is bounded by the common ancestor of the current working directory
and the user-global config directory, which keeps inheritance local to the
user's workspace while still supporting nested projects and worktrees.

Two additional layers influence discovery:

1. **XDG environment variables** -- override OS-native defaults for user-level
   config, data, cache, and state directories.
2. **Entry point detection** -- the binary's invocation mode (standalone vs hop)
   determines which filenames and directory prefixes the walk-up searches for.

## XDG Environment Variables

**File**: `internal/config/paths.go`

TLC honours the [XDG Base Directory Specification](
https://specifications.freedesktop.org/basedir-spec/latest/) on all
platforms. When an XDG variable is set it takes precedence over the
OS-native default; when unset, the platform convention applies.

**`XDG_CONFIG_HOME`** -- `$XDG_CONFIG_HOME/tlc/`
- macOS: `~/Library/Application Support/tlc/`
- Linux: `~/.config/tlc/`

**`XDG_DATA_HOME`** -- `$XDG_DATA_HOME/tlc/`
- macOS: `~/Library/Application Support/tlc/`
- Linux: `~/.local/share/tlc/`

**`XDG_CACHE_HOME`** -- `$XDG_CACHE_HOME/tlc/`
- macOS: `~/Library/Caches/tlc/`
- Linux: `~/.cache/tlc/`

**`XDG_STATE_HOME`** -- `$XDG_STATE_HOME/tlc/`
- macOS: `~/Library/Application Support/tlc/state/`
- Linux: `~/.local/state/tlc/`

Resolution logic (pseudocode, same pattern for each directory kind):

```
if $XDG_<KIND>_HOME is set:
    return $XDG_<KIND>_HOME/tlc/
else:
    return <os-native-default>/tlc/
```

The user-global config file used by Viper at Stage 1 lives at
`UserConfigDir()/config.yaml`. Setting `XDG_CONFIG_HOME` therefore
moves this file, which in turn shifts the **walk-up boundary**
computed by `resolveProjectConfigBoundary()`.

---

## Entry Point Detection (Mode)

**File**: `internal/config/mode.go`

Before the walk-up begins, `initConfig()` calls `config.DetectMode()`
to decide whether TLC is running **standalone** or as a **hop**
integration. The mode controls which filenames the walk-up probes
at each directory level.

### Detection Algorithm

```
1. If $TLC_MODE is set:
     "hop"        -> ModeHop
     "standalone" -> ModeStandalone

2. If cwd path contains a ".hop" segment -> ModeHop

3. Walk up from cwd; if any ancestor has a hop config
   (.hop/tlc/ directory or .hop/tlc.yaml file) -> ModeHop

4. Otherwise -> ModeStandalone
```

A `.hop/` directory that holds neither `.hop/tlc/` nor `.hop/tlc.yaml`
is **not** a hop indicator. Repair tooling leaves a bare `.hop/`
(`repair.lock`, `backups/`) in hubs that were initialized standalone;
those keep loading `.tlc/` and `tlc init` there scaffolds `.tlc/`.

### Mode-Specific Config Filenames

| Mode | Flat File | Directory Config |
|---|---|---|
| **Standalone** | `.tlc.yaml` | `.tlc/config.yaml` |
| **Hop** | `.hop/tlc.yaml` | `.hop/tlc/config.yaml` |

These are returned by `LocalConfigFile(mode)` and
`LocalConfigDir(mode)` respectively, and fed directly into
`findAllConfigsForMode()`.

### Validation

After mode detection, `ValidateLocalConfig(mode, cwd)` walks up
from cwd to confirm at least one matching config exists. In hop
mode, a missing config produces a warning; in standalone mode it
is silently accepted (user/system config may still apply).

---

## Problem with Viper Native Search

### Viper's Static Path Search

Viper provides a static configuration path search:

```go
// Viper only checks these exact paths:
viper.AddConfigPath("/etc/tlc")                    // System-wide
viper.AddConfigPath("$HOME/.config/tlc")          // User home (Linux)
viper.SetConfigName("config")                       // Looks for "config.yaml"

// When you call viper.ReadInConfig(), it only checks:
// /etc/tlc/config.yaml
// <os user config dir>/tlc/config.yaml
```

### Limitations

| Use Case | Viper Native | Why It Fails |
|----------|--------------|---------------|
| **Nested projects** (`/repo/service-a/`) | ❌ Doesn't search parent directories from CWD | Doesn't know about project hierarchy |
| **Multi-clone workflows** (`~/repo-a/` vs `~/repo-b/`) | ❌ Can't find sibling configs | Only checks specific system paths |
| **Git worktrees** (`/.worktrees/feature/`) | ❌ Doesn't search worktree directories | Static paths don't include worktrees |
| **Arbitrary project locations** (`/any/path/to/project/`) | ❌ Can't discover from CWD | Not relative to working directory |

## TLC's Custom Walk-Up Implementation

### Location

**File**: `internal/cli/root.go`

### Function Signatures

```go
// Legacy convenience wrapper; delegates to findAllConfigsForMode
// using the auto-detected mode.
func findAllConfigs(startDir, stopDir string) []string

// Mode-aware walk-up (the real implementation).
func findAllConfigsForMode(
    startDir, stopDir string, mode config.EntryMode,
) []string
```

### Algorithm

The function implements a **bounded, mode-aware bottom-up traversal**.
The filenames probed at each level depend on the active `EntryMode`:

```
findAllConfigsForMode(startDir, stopDir, mode):
    flatFile  = LocalConfigFile(mode)   // e.g. .tlc.yaml | .hop/tlc.yaml
    dirConfig = LocalConfigDir(mode) + "/config.yaml"
                                        // e.g. .tlc/config.yaml
                                        //    | .hop/tlc/config.yaml
    curr = normalize(startDir)

    loop:
        stat(curr / flatFile)    -> append if exists
        stat(curr / dirConfig)   -> append if exists

        if curr == stopDir -> break
        parent = filepath.Dir(curr)
        if parent == curr  -> break   // filesystem root
        curr = parent

    return configs
```

### Key Characteristics

1. **Dynamic Starting Point**: Begins from current working directory
   (`os.Getwd()`)
2. **Hierarchical Traversal**: Moves up one directory at a time
3. **Mode-Dependent Filenames**: Probes standalone or hop paths based
   on the detected `EntryMode`
4. **Multiple Config Formats**: Flat file and directory config checked
   at every level
5. **Termination Condition**: Stops at the computed boundary directory,
   or filesystem root if the common ancestor is `/`
6. **Return Order**: Configs collected from CWD outward; the merge loop
   iterates in reverse so root-most is loaded first, closest wins

## How TLC Uses the Walk-Up

### Config Loading Flow

**File**: `internal/cli/root.go`

```
initConfig():
    setDefaults()

    // Stage 1 -- global/system configs
    viper.AddConfigPath(UserConfigDir())   // XDG or OS default
    viper.AddConfigPath("/etc/tlc")
    viper.SetConfigName("config")
    viper.ReadInConfig()

    // Stage 2 -- detect mode, then walk up with mode-specific names
    mode = config.DetectMode()
    config.ValidateLocalConfig(mode, cwd)

    configs = findAllConfigsForMode(
        cwd,
        resolveProjectConfigBoundary(cwd),
        mode,
    )

    // Merge root-most first so closest wins
    for i = len(configs)-1 .. 0:
        viper.MergeInConfig(configs[i])

    // Stage 3 -- env vars override everything
    viper.SetEnvPrefix("TLC")
    viper.AutomaticEnv()
```

### Config Cascade with Mode Awareness

Configs merge in this order (later overrides earlier). File paths
shown use **standalone** names; substitute `.hop/tlc.yaml` and
`.hop/tlc/config.yaml` when mode is **hop**.

1. `/etc/tlc/config.yaml` (system)
2. `<UserConfigDir>/config.yaml` (user -- XDG or OS default)
3. `{boundary}/.tlc/config.yaml` (common-ancestor boundary)
4. `/repo/.tlc.yaml` or `/repo/.tlc/config.yaml` (root-most match)
5. `/repo/sub/.tlc.yaml` or `/repo/sub/.tlc/config.yaml` (closer)
6. `/repo/sub/cwd/.tlc.yaml` or `/repo/sub/cwd/.tlc/config.yaml`
   (closest -- CWD)
7. Environment variables (`TLC_*`)

In **hop** mode the same cascade applies but every probe uses
`.hop/tlc.yaml` and `.hop/tlc/config.yaml` instead of the `.tlc`
variants. When hop mode was forced (`TLC_MODE=hop`) or inherited and
the walk finds no hop config at all, the standalone probes run instead,
so a project whose only config is `.tlc/` keeps reading its own store.

### Both Layouts Present

A directory may carry both `.tlc/` and `.hop/tlc/` (hubs commonly
mirror one into the other). Each directory on the walk is checked:

- Same `project.id` **and** same absolute `storage.db_path` (both
  unset counts as equal): the mode's copy loads, debug-level log only.
- Any difference: the command exits with code 4 (conflict) naming both
  files and both `(project.id, db_path)` pairs. Make the files agree or
  remove one; tlc never lists zero rows silently in this state.

## Example Scenarios

### Scenario 1: Nested Monorepo

**Directory Structure**:
```
/monorepo/
  .tlc/config.yaml                 ← Root project config
  services/
    api/
      .tlc/config.yaml           ← API service config
    web/
      .tlc/config.yaml           ← Web service config
      ← Current working directory
```

**Walk-Up Process**:

1. Start at `/monorepo/services/web/`
2. Check `.tlc.yaml` → not found
3. Check `.tlc/config.yaml` → **FOUND** → Add to list
4. Move up to `/monorepo/services/`
5. Check `.tlc.yaml` → not found
6. Check `.tlc/config.yaml` → not found
7. Move up to `/monorepo/`
8. Check `.tlc.yaml` → not found
9. Check `.tlc/config.yaml` → **FOUND** → Add to list
10. Move up to `~/` (the common ancestor with `~/.config/tlc`)
11. Stop

**Config List Returned**:
```go
[
    "/monorepo/.tlc/config.yaml",      // Root-most
    "/monorepo/services/web/.tlc/config.yaml",  // Closest
]
```

**Merge Order** (root-most to closest):
1. Viper loads `/monorepo/.tlc/config.yaml` (base settings)
2. Viper merges `/monorepo/services/web/.tlc/config.yaml` (overrides api-specific)
3. Environment variables apply on top

**Result**: API service inherits root config but can override settings locally

---

### Scenario 2: Git Worktree

**Directory Structure**:
```
~/my-repo/
  .tlc/config.yaml                 ← Main branch config
  .worktrees/
    feature-login/
      .tlc/config.yaml           ← Worktree-specific config
      ← Current working directory
```

**Walk-Up Process**:

1. Start at `~/my-repo/.worktrees/feature-login/`
2. Check `.tlc.yaml` → not found
3. Check `.tlc/config.yaml` → **FOUND** → Add to list
4. Move up to `~/my-repo/.worktrees/`
5. Check `.tlc.yaml` → not found
6. Check `.tlc/config.yaml` → not found
7. Move up to `~/my-repo/`
8. Check `.tlc.yaml` → not found
9. Check `.tlc/config.yaml` → **FOUND** → Add to list
10. Move up to `~/`
11. Check `.tlc.yaml` → not found
12. Check `.tlc/config.yaml` → not found
13. Move up to `~/` (the common ancestor with `~/.config/tlc`)
14. Stop

**Config List Returned**:
```go
[
    "/home/user/my-repo/.tlc/config.yaml",              // Root-most (main)
    "/home/user/my-repo/.worktrees/feature-login/.tlc/config.yaml",  // Closest (worktree)
]
```

**Merge Order**:
1. Viper loads main branch config
2. Viper merges worktree config (overrides main settings)
3. Environment variables apply on top

**Result**: Worktree can have different task settings than main branch

---

### Scenario 3: Multi-Clone Workflow (Current Problem)

**Directory Structure**:
```
~/projects/
  repo-a/
    .tlc/config.yaml                 ← Clone A's config
    ← Previously initialized here
  repo-b/
    ← Current working directory (Clone B, no .tlc)
```

**Walk-Up Process**:

1. Start at `~/projects/repo-b/`
2. Check `.tlc.yaml` → not found
3. Check `.tlc/config.yaml` → not found
4. Move up to `~/projects/`
5. Check `.tlc.yaml` → not found
6. Check `.tlc/config.yaml` → not found
7. Move up to `~/`
8. Check `.tlc.yaml` → not found
9. Check `.tlc/config.yaml` → not found
10. Move up to `~/` (the common ancestor with `~/.config/tlc`)
11. Stop

**Config List Returned**:
```go
[]  // Empty!
```

**Viper State After Load**:
- Global configs loaded (if any)
- No project-specific config loaded
- `viper.ConfigFileUsed()` returns: `""`

**Result**: `DetectProject()` receives no config path and returns `InProject: false`

---

### Scenario 4: Multi-Clone Workflow (With Git Remote Fallback)

**After implementing the planned git remote fallback**:

**Directory Structure**:
```
~/projects/
  repo-a/
    .tlc/config.yaml                 ← Has project.id: github.com/user/repo
  repo-b/
    ← Git repo with same remote: github.com/user/repo
    ← No .tlc/ directory
```

**Planned Flow**:

1. `findAllConfigs("~/projects/repo-b/")` returns empty
2. `viper.ConfigFileUsed()` returns `""`
3. **NEW**: `DetectProject()` detects this situation
4. **NEW**: Falls back to git remote detection
5. **NEW**: Infers `project_id: github.com/user/repo` from `git remote get-url origin`
6. Returns `ProjectDetection{
    ProjectID: "github.com/user/repo",
    ConfigPath: "",  // No local config yet
    InProject: true,   // ← STILL considers us in a project!
}`

**Result**: Clone B automatically uses the same project as Clone A
without creating `.tlc/config.yaml`

---

## Why This Matters for Git Remote Fallback

### The Gap

The custom walk-up **works perfectly** for finding existing `.tlc/` configs:

| Scenario | Walk-Up Finds Config? | Works Today |
|----------|-----------------------|------------|
| Nested monorepo | ✅ Yes | ✅ Yes |
| Git worktree | ✅ Yes | ✅ Yes |
| Multi-clone (has .tlc) | ✅ Yes | ✅ Yes |
| **Multi-clone (NO .tlc)** | ❌ No | ❌ **NO** |

### The Solution Connection

The **git remote fallback** feature connects to the walk-up:

```
Walk-Up Flow:
  1. Start at CWD
  2. Search upward for .tlc/
  3. Return config paths (may be empty)
       ↓
       ↓
DetectProject():
  1. Get merged config from Viper
  2. Check viper.ConfigFileUsed()
  3. If empty → call git remote fallback
  4. Detect project_id from git remote
  5. Return InProject: true with inferred ID
```

**Key Insight**: 
- Walk-up handles **filesystem-based config discovery** (finding `.tlc/` dirs)
- Git remote fallback handles **git-based project identity** (when no `.tlc/` exists)
- Both work together to provide seamless experience across clones

---

## Comparison Table

| Aspect | Viper Native | TLC Walk-Up | Combined (with Remote Fallback) |
|--------|--------------|--------------|--------------------------------|
| **Static paths only** | ✅ | ❌ | ✅ Uses static as base |
| **Dynamic from CWD** | ❌ | ✅ | ✅ |
| **Parent directory search** | ❌ | ✅ | ✅ |
| **Worktree support** | ❌ | ✅ | ✅ |
| **Multi-clone support** | ❌ | ✅ | ✅ |
| **Git remote inference** | ❌ | ❌ | ✅ NEW |
| **Environment variables** | ✅ | ✅ | ✅ |

---

## Implementation Notes

### Performance Considerations

1. **Filesystem Calls**: Each directory level performs 2 `os.Stat()` calls
2. **Traversal Depth**: Typically 5-10 levels deep (from project to the computed boundary)
3. **Total Calls**: 10-20 syscalls per TLC invocation (negligible)

### Edge Cases Handled

1. **Bounded walk-up**: Stops at the common ancestor of `cwd` and the user-global config
   directory, or at filesystem root when that ancestor is `/`
2. **Multiple config files**: Same directory can have both `.tlc.yaml` and `.tlc/config.yaml`
3. **Permission errors**: `os.Stat()` errors are silently ignored (file doesn't exist)
4. **Symlinks**: `filepath.Dir()` and `os.Stat()` work with symlinks naturally

### Error Handling

```go
// No error propagation - missing files are expected
if _, err := os.Stat(tlcYaml); err == nil {
    // File exists
}
// If err != nil, it's "file not found" - continue searching
```

This is intentional: Missing config files are normal, not errors.

---

## Future Considerations

### Potential Optimizations

1. **Caching**: Cache `findAllConfigs()` result per invocation (already called once)
2. **Early Exit**: Stop at git repo root if `.git/` found (optional)
3. **Config Inheritance**: Explicit `extends:` field to reference parent configs

### Alternative Approaches (Not Used)

1. **Viper search paths**: `viper.AddConfigPath(".")` doesn't walk up
2. **Custom Viper provider**: Write a provider that implements walk-up
   - Pros: Cleaner Viper integration
   - Cons: More complex, loses explicit control

Current approach chosen for: **Simplicity, explicit control, easy to debug**

---

## References

- **Walk-up & merge loop**: `internal/cli/root.go`
- **XDG / path resolution**: `internal/config/paths.go`
- **Entry mode detection**: `internal/config/mode.go`
- **Project detection**: `internal/core/project.go`
- **Git Remote Fallback**: `docs/IMPLEMENTATION_PLAN_remote_fallback_and_duplicate_handling.md`
