# Multi-Clone Setup Guide

When you work with the same repository in multiple locations (e.g., different feature branches, worktrees, or machines), TLC provides flexible options for managing tasks across these clones.

## Understanding Project IDs

Every TLC project has a unique `project_id` that groups tasks together. When working with clones:

- **Same project_id**: Tasks appear in all clones (shared mode)
- **Different project_ids**: Tasks are isolated per clone (unique mode)

## Duplicate Project ID Strategies

When you run `tlc init` in a clone where the `project_id` already exists, you have three strategies:

### Share Strategy (Default)

All clones share the same tasks.

```bash
tlc init --duplicate-id-strategy share
```

**Use Case**: You want task visibility across all clones.

Example:
```bash
# Clone A (work laptop)
cd ~/repo-a
tlc init
tlc task create "Feature X"
# Project ID: user/repo (created from git remote)

# Clone B (home computer)
cd ~/repo-b
tlc init --duplicate-id-strategy share
# Same project ID: user/repo
# Feature X task is visible here too
```

**Result**: Both clones see and can modify the same tasks.

### Unique Strategy

Each clone gets its own unique project ID.

```bash
tlc init --duplicate-id-strategy unique
```

**Use Case**: You want isolated task lists per clone.

Example:
```bash
# Clone A
cd ~/repo-main
tlc init
tlc task create "Main feature"
# Project ID: user/repo

# Clone B (feature branch)
cd ~/repo-feature
tlc init --duplicate-id-strategy unique
# Project ID: user/repo-feature (auto-generated with directory name)
tlc task create "Experiment feature"
# This task only appears in the feature branch clone
```

**Result**: Tasks created in clone B are not visible in clone A.

**Unique ID Generation**:

TLC tries these strategies to generate a unique ID:

1. Directory suffix: `user/repo-feature-branch`
2. Numeric suffix: `user/repo-2`, `user/repo-3`, etc.
3. Timestamp suffix: `user/repo-20250121` (last resort)

### Prompt Strategy

Ask the user what to do.

```bash
tlc init --duplicate-id-strategy prompt
```

Example:
```bash
cd ~/repo-clone-2
tlc init --duplicate-id-strategy prompt
# Interactive prompt appears:
#
#   Project 'user/repo' already exists with 15 tasks
#   How should TLC handle this clone?
#
#   > Share existing project (recommended)
#     Create unique project for this clone
```

**Use Case**: Want control on a per-clone basis.

## Common Scenarios

### Scenario 1: Feature Branch Worktrees

You want isolated tasks for each feature branch.

**Setup**:
```bash
cd ~/main-repo
tlc init
git worktree add ../feature-A feature-A
cd ../feature-A
tlc init --duplicate-id-strategy unique
```

**Result**: Each worktree has its own task list.

### Scenario 2: Multiple Machines

Same repo on multiple machines, want shared tasks.

**Setup**:
```bash
# Machine A (work laptop)
cd ~/work-repo
tlc init --duplicate-id-strategy share

# Machine B (home computer)
cd ~/home-repo
tlc init --duplicate-id-strategy share
```

**Result**: Both machines access same task list (assuming same database).

**Note**: For truly shared tasks, use a shared database backend like PostgreSQL.

### Scenario 3: CI/CD Environments

CI runners should use detected mode without creating config files.

**Setup**:
```yaml
# .github/workflows/ci.yml
env:
  TLC_PROJECT_DUPLICATE_ID_STRATEGY: share
  TLC_PROJECT_FALLBACK_MODE: detected
steps:
  - name: Run TLC tasks
    run: |
      tlc task list --all-projects
```

**Result**: CI automatically detects project from git remote without persistent config.

### Scenario 4: Temporary Experimental Clones

Create a clone for quick experiments.

**Setup**:
```bash
cd ~/exp-repo
tlc init --duplicate-id-strategy unique --fallback-mode detected
```

**Result**:
- Unique project ID for experimental work
- No config file created (detected mode)
- Tasks isolated from main repo

## Migration from Old Setups

If you have existing TLC setups with clones sharing tasks:

1. **Current behavior** (no change needed): Clones automatically share tasks
2. **To isolate**: Run `tlc init --duplicate-id-strategy unique --force`
3. **To switch strategies**: Edit `.tlc/config.yaml`:
   ```yaml
   project:
     duplicate_id_strategy: unique
   ```

## Viewing Tasks Across Projects

When using unique strategies, view tasks from all clones:

```bash
# Show tasks from all projects
tlc task list --all-projects

# Filter by specific project
tlc task list --project user/repo-2

# Show which project each task belongs to
tlc task list --format table --columns id,status,description,project_id
```

## Configuration Best Practices

### For Shared Development Teams

```yaml
# .tlc/config.yaml (checked into git)
project:
  fallback_mode: auto
  duplicate_id_strategy: share
```

All team members share the same tasks.

### For Solo Developers with Multiple Branches

```yaml
# .tlc/config.yaml
project:
  fallback_mode: auto
  duplicate_id_strategy: unique
```

Each branch gets isolated tasks.

### For CI/CD Pipelines

```yaml
# Global config (e.g., /etc/tlc/config.yaml)
project:
  fallback_mode: detected
  duplicate_id_strategy: share
```

No config files created in CI environments.

## Troubleshooting

### Tasks Not Appearing Across Clones

**Problem**: You created a task in clone A, but it doesn't appear in clone B.

**Solutions**:

1. Check both clones have the same `project_id`:
   ```bash
   # In clone A
   tlc config get project.id

   # In clone B
   tlc config get project.id

   # They should match for shared tasks
   ```

2. Verify duplicate strategy:
   ```bash
   # In clone B
   cat .tlc/config.yaml | grep duplicate_id_strategy
   ```

3. If using different databases, sync them:
   ```bash
   tlc sync export --all-projects > backup.json
   tlc sync import backup.json
   ```

### Wrong Project ID Generated

**Problem**: TLC generated `user/repo-2` when you wanted `user/repo`.

**Solution**: Manually set the project ID:
```yaml
# .tlc/config.yaml
project:
  id: "user/repo"
```

Then reinitialize:
```bash
tlc init --force
```

### Confusion Between Clones

**Problem**: Hard to remember which clone has which tasks.

**Solution**: Use descriptive clone names or document your strategy:

```bash
# Clone naming convention
~/repo-main/        # Main development
~/repo-feature-x/    # Feature X experiments
~/repo-bug-fix/     # Bug fixes
```

Or use environment-specific configs:
```yaml
# In .tlc/config.yaml for each clone
project:
  id: "user/repo-main-feature"    # For main clone
  # id: "user/repo-feature-x"     # For feature X clone
```

## Advanced: Linking Projects

If you initially chose "unique" but later want to share:

1. Check current project IDs:
   ```bash
   tlc task list --all-projects --format json | jq -r '.[].project_id' | sort -u
   ```

2. Update config to share:
   ```yaml
   # .tlc/config.yaml
   project:
     id: "user/repo"  # Set to the project you want to join
     duplicate_id_strategy: share
   ```

3. Reinitialize:
   ```bash
   tlc init --force
   ```

4. Tasks will now be shared.

**Note**: Old tasks from the unique project won't automatically merge. Use sync commands to export/import if needed.
