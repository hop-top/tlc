# TLC Config Discovery: Custom Walk-Up Implementation

## Overview

TLC uses a **custom directory traversal** (`findAllConfigs()`) to discover project
configuration files, rather than relying on Viper's static path search. The walk-up is
bounded by the common ancestor of the current working directory and the user-global
config directory, which keeps inheritance local to the user's workspace while still
supporting nested projects and worktrees.

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

**File**: `internal/cli/root.go:131-155`

### Function Signature

```go
func findAllConfigs(startDir string) []string
```

### Algorithm

The function implements a **bounded bottom-up directory traversal**:

```go
func findAllConfigs(startDir, stopDir string) []string {
    var configs []string
    curr := startDir
    
    for {
        // 1. Check for .tlc.yaml in current directory
        tlcYaml := filepath.Join(curr, ".tlc.yaml")
        if _, err := os.Stat(tlcYaml); err == nil {
            configs = append(configs, tlcYaml)
        }
        
        // 2. Check for .tlc/config.yaml in current directory
        tlcDirConfig := filepath.Join(curr, ".tlc", "config.yaml")
        if _, err := os.Stat(tlcDirConfig); err == nil {
            configs = append(configs, tlcDirConfig)
        }
        
        if stopDir != "" && curr == stopDir {
            break
        }

        // 3. Move up to parent directory
        parent := filepath.Dir(curr)
        if parent == curr {  // Reached filesystem root
            break
        }
        curr = parent
    }
    
    return configs
}
```

### Key Characteristics

1. **Dynamic Starting Point**: Begins from current working directory (`os.Getwd()`)
2. **Hierarchical Traversal**: Moves up one directory at a time
3. **Multiple Config Formats**: Supports both `.tlc.yaml` and `.tlc/config.yaml`
4. **Termination Condition**: Stops when the walk reaches the computed boundary
   directory, or filesystem root if the common ancestor is `/`
5. **Return Order**: Configs collected from root-most to closest, later reversed

## How TLC Uses the Walk-Up

### Config Loading Flow

**File**: `internal/cli/root.go:53-91`

```go
func initConfig() {
    setDefaults()
    
    // Stage 1: Load global/system configs
    viper.AddConfigPath("/etc/tlc")
    viper.AddConfigPath(filepath.Join(home, ".config", "tlc"))
    viper.SetConfigName("config")
    viper.ReadInConfig()
    
    // Stage 2: Walk up from current directory up to the common ancestor
    // of cwd and the user-global config directory
    curr, _ := os.Getwd()
    configs := findAllConfigs(curr, resolveProjectConfigBoundary(curr))
    
    // Merge them from root-most to closest
    // so that closer files overwrite further ones
    for i := len(configs) - 1; i >= 0; i-- {
        viper.SetConfigFile(configs[i])
        viper.MergeInConfig()
    }
    
    // Stage 3: Environment variables override all
    viper.SetEnvPrefix("TLC")
    viper.AutomaticEnv()
}
```

### Merge Order

Configs are merged in this order (later configs override earlier ones):

1. `/etc/tlc/config.yaml` (system)
2. `<os user config dir>/tlc/config.yaml` (user home)
3. `{boundary}/.tlc/config.yaml` (if present at the common-ancestor boundary)
4. `/repo/.tlc/config.yaml` (root-most found below the boundary)
5. `/repo/subproject/.tlc/config.yaml` (closer found by walk-up)
6. `/repo/subproject/feature/.tlc/config.yaml` (closest - CWD)
7. Environment variables (`TLC_*`)

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

**Result**: Clone B automatically uses the same project as Clone A without creating `.tlc/config.yaml`

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
2. **Environment paths**: `XDG_CONFIG_DIRS` - complex cross-platform handling
3. **Custom Viper provider**: Write a Viper provider that implements walk-up
   - Pros: Cleaner Viper integration
   - Cons: More complex, loses explicit control

Current approach chosen for: **Simplicity, explicit control, easy to debug**

---

## References

- **Implementation**: `internal/cli/root.go`
- **Usage**: `internal/cli/root.go:78-90` (merge loop)
- **Related**: `internal/core/project.go` (uses merged config)
- **Git Remote Fallback**: `docs/IMPLEMENTATION_PLAN_remote_fallback_and_duplicate_handling.md`
