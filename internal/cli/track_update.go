package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
)

var trackUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a track",
	Long: `Update a track's title, status, assignee, type, due date, or append
a plan file.

Status transitions follow the TrackService state machine. --add-plan
attaches an additional planning document to the track's plan directory.`,
	Annotations: map[string]string{
		"kit/side-effect": "write-local",
	},
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := getStorageRaw()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := context.Background()
		id, err := resolveTrackID(ctx, s, args[0])
		if err != nil {
			return err
		}

		svc := core.NewTrackService(s, s)

		titleChanged := cmd.Flags().Changed("title")
		statusChanged := cmd.Flags().Changed("status")
		assignedChanged := cmd.Flags().Changed("assigned-to")
		typeChanged := cmd.Flags().Changed("type")
		addPlanChanged := cmd.Flags().Changed("add-plan")
		dueChanged := cmd.Flags().Changed("due")

		if !titleChanged && !statusChanged && !assignedChanged &&
			!typeChanged && !addPlanChanged && !dueChanged {
			return fmt.Errorf(
				"no update flags provided; use --title, --status, " +
					"--assigned-to, --type, --add-plan, or --due",
			)
		}

		cfgTypes := getConfigTrackTypes()
		if typeChanged && !core.ValidTrackType(trackUpdateType, cfgTypes) {
			return fmt.Errorf(
				"track type %q invalid; valid types: %s",
				trackUpdateType, core.TrackTypeList(cfgTypes),
			)
		}

		w := cmd.OutOrStdout()

		// Handle --add-plan before the regular update so the plan link
		// is part of the same track mutation.
		var planFm *core.PlanFrontmatter
		if addPlanChanged && trackUpdateAddPlan != "" {
			fm, parseErr := core.ParsePlanFrontmatter(trackUpdateAddPlan)
			if parseErr != nil {
				return parseErr
			}
			planFm = fm

			// Guard against the silent track-mismatch bug (T-0857): if
			// the plan's frontmatter declares tracks: [...], every entry
			// must reference the track the user passed on the CLI.
			// Otherwise the CLI-supplied id wins (per scope) but only
			// after we surface the conflict instead of silently routing
			// tasks to the wrong place.
			if fm != nil && len(fm.Tracks) > 0 {
				track, getErr := s.GetTrack(ctx, id)
				if getErr != nil {
					return fmt.Errorf(
						"get track %q for plan validation: %w", id, getErr,
					)
				}
				if track == nil {
					return fmt.Errorf(
						"track %q not found; run 'tlc track list' to "+
							"see available tracks", id,
					)
				}
				for _, ref := range fm.Tracks {
					ref = strings.TrimSpace(ref)
					if ref == "" {
						continue
					}
					if ref == track.ID || ref == track.Slug {
						continue
					}
					return fmt.Errorf(
						"plan %q frontmatter tracks: [%s] does not "+
							"match target track %q (slug %q); "+
							"re-run with the matching track id or "+
							"fix the plan frontmatter",
						trackUpdateAddPlan,
						strings.Join(fm.Tracks, ", "),
						track.ID, track.Slug,
					)
				}
			}
		}

		// Parse --due ahead of the mutation closure so an invalid value
		// fails before any write. The "" / "-" sentinels clear the field
		// (mirrors task update); anything else goes through util.ParseUntil
		// per the temporal spec (UTC RFC3339 on the wire).
		var (
			dueClear  bool
			parsedDue *time.Time
		)
		if dueChanged {
			raw := trackUpdateDue
			if raw == "" || raw == "-" {
				dueClear = true
			} else {
				dt, perr := util.ParseUntil(raw)
				if perr != nil {
					return fmt.Errorf("invalid --due %q: %w", raw, perr)
				}
				parsedDue = &dt
			}
		}

		err = svc.UpdateTrack(ctx, id, func(t *core.Track) error {
			if titleChanged {
				t.Title = trimMatchingQuotes(trackUpdateTitle)
			}
			if statusChanged {
				t.Status = core.TrackStatus(trackUpdateStatus)
			}
			if assignedChanged {
				if trackUpdateAssignedTo == "" ||
					trackUpdateAssignedTo == "-" ||
					trackUpdateAssignedTo == "null" {
					t.AssignedTo = nil
				} else {
					a := normalizeAssignee(trackUpdateAssignedTo)
					t.AssignedTo = &a
				}
			}
			if typeChanged {
				t.Type = trackUpdateType
			}
			if addPlanChanged && trackUpdateAddPlan != "" {
				linkPlanToTrack(t, trackUpdateAddPlan)
			}
			if dueChanged {
				if dueClear {
					t.DueAt = nil
				} else {
					t.DueAt = parsedDue
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		displayID := trackDisplayID(ctx, svc, id)
		_, _ = fmt.Fprintf(w, "Updated track %s\n", displayID)

		// Resolve task specs: frontmatter first, then extractor fallback.
		var taskSpecs []core.PlanTaskSpec
		if planFm != nil && len(planFm.Tasks) > 0 {
			taskSpecs = planFm.Tasks
		} else if addPlanChanged && trackUpdateAddPlan != "" {
			// Try configured plan extractor.
			extCmd := viper.GetString("tracks.plan_extractor")
			if extCmd != "" {
				extracted, extErr := core.RunPlanExtractor(
					extCmd, trackUpdateAddPlan,
				)
				if extErr != nil {
					_, _ = fmt.Fprintf(
						w, "Warning: extractor failed: %v\n", extErr,
					)
				} else {
					taskSpecs = extracted
				}
			}
		}

		if len(taskSpecs) > 0 || (addPlanChanged && trackUpdateAddPlan != "") {
			var projectID string
			if proj := core.DetectProject(); proj != nil &&
				proj.ProjectID != "" {
				projectID = proj.ProjectID
			}

			// Check if track already has a plan mapping (re-run).
			track, getErr := svc.GetTrack(ctx, id)
			if getErr != nil {
				return fmt.Errorf("get track for reconciliation: %w", getErr)
			}

			if len(track.PlanMapping) > 0 {
				// Reconcile: idempotent re-run.
				rec, recErr := svc.ReconcileTasksFromPlan(
					ctx, id, taskSpecs, projectID, s,
					track.PlanMapping,
				)
				if recErr != nil {
					return fmt.Errorf(
						"plan linked but reconciliation failed: %w",
						recErr,
					)
				}
				_, _ = fmt.Fprintf(w, "Reconciled plan for track %s:\n", displayID)
				_, _ = fmt.Fprintf(w, "  Created: %d tasks\n", len(rec.Created))
				_, _ = fmt.Fprintf(w, "  Updated: %d tasks\n", len(rec.Updated))
				_, _ = fmt.Fprintf(w, "  Unchanged: %d tasks\n", len(rec.Unchanged))
				_, _ = fmt.Fprintf(w, "  Deleted: %d tasks (TODO)\n", len(rec.Deleted))
				if len(rec.Kept) > 0 {
					_, _ = fmt.Fprintf(
						w, "  Kept: %d tasks (non-TODO, removed from plan)\n",
						len(rec.Kept),
					)
				}

				// Cross-project refs cannot be resolved locally. Say so
				// rather than letting the edge disappear behind an
				// "Unchanged" summary.
				for _, d := range rec.DeferredCrossProject {
					_, _ = fmt.Fprintf(
						w,
						"Warning: task %s blocked-by %q is cross-project "+
							"and stays deferred until that project is "+
							"available\n",
						d.TaskID, d.Ref,
					)
				}

				// Phase 2: resolve pending cross-track refs after
				// reconciliation, same as first-run path.
				phase2, p2Err := svc.ResolvePendingCrossTrackRefs(
					ctx, projectID, nil,
				)
				if p2Err != nil {
					return fmt.Errorf("phase 2 resolution: %w", p2Err)
				}
				if phase2.PromotedTasks > 0 {
					_, _ = fmt.Fprintf(
						w, "Resolved deferred refs on %d tasks\n",
						phase2.PromotedTasks,
					)
				}
				for _, p := range phase2.PlansRewritten {
					_, _ = fmt.Fprintf(
						w, "Rewrote plan %s with resolved refs\n", p,
					)
				}
				if len(phase2.StillUnresolved) > 0 {
					for _, u := range phase2.StillUnresolved {
						_, _ = fmt.Fprintf(
							w,
							"Warning: task %s still waiting on %q\n",
							u.TaskID, u.Ref,
						)
					}
				}
			} else if len(taskSpecs) > 0 {
				// First-time ingest.
				result, createErr := svc.CreateTasksFromPlan(
					ctx, id, taskSpecs, projectID, s,
				)
				if createErr != nil {
					return fmt.Errorf(
						"plan linked but task creation failed: %w",
						createErr,
					)
				}
				_, _ = fmt.Fprintf(
					w, "Linked plan %s, created %d tasks\n",
					trackUpdateAddPlan, len(result.CreatedIDs),
				)

				// Phase 2: resolve any pending cross-track refs.
				seed := map[string]map[string]string{
					trackUpdateAddPlan: result.ResolvedRefs,
				}
				phase2, p2Err := svc.ResolvePendingCrossTrackRefs(
					ctx, projectID, seed,
				)
				if p2Err != nil {
					return fmt.Errorf("phase 2 resolution: %w", p2Err)
				}
				if phase2.PromotedTasks > 0 {
					_, _ = fmt.Fprintf(
						w, "Resolved deferred refs on %d tasks\n",
						phase2.PromotedTasks,
					)
				}
				for _, p := range phase2.PlansRewritten {
					_, _ = fmt.Fprintf(
						w, "Rewrote plan %s with resolved refs\n", p,
					)
				}
				if len(result.UnresolvedRefs) > 0 {
					_, _ = fmt.Fprintf(
						w,
						"Warning: %d unresolved cross-track refs in "+
							"%s (will retry on next ingest): %s\n",
						len(result.UnresolvedRefs),
						trackUpdateAddPlan,
						strings.Join(result.UnresolvedRefs, ", "),
					)
				}
				if len(phase2.StillUnresolved) > 0 {
					for _, u := range phase2.StillUnresolved {
						_, _ = fmt.Fprintf(
							w,
							"Warning: task %s still waiting on %q\n",
							u.TaskID, u.Ref,
						)
					}
				}
			} else {
				_, _ = fmt.Fprintf(
					w, "Linked plan %s (no tasks extracted)\n",
					trackUpdateAddPlan,
				)
			}
		}

		return nil
	},
}

// linkPlanToTrack appends planPath to track.Meta["plans"] if not already
// present.
func linkPlanToTrack(t *core.Track, planPath string) {
	if t.Meta == nil {
		t.Meta = make(map[string]any)
	}

	var plans []string
	if existing, ok := t.Meta["plans"]; ok {
		switch v := existing.(type) {
		case []string:
			plans = v
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok {
					plans = append(plans, s)
				}
			}
		}
	}

	// Deduplicate.
	for _, p := range plans {
		if p == planPath {
			return
		}
	}

	plans = append(plans, planPath)
	t.Meta["plans"] = plans
}

func init() {
	trackUpdateCmd.Flags().StringVar(
		&trackUpdateTitle, "title", "", "New track title",
	)
	trackUpdateCmd.Flags().StringVar(
		&trackUpdateStatus, "status", "",
		"New status (pending, active, completed, abandoned, archived)",
	)
	trackUpdateCmd.Flags().StringVar(
		&trackUpdateAssignedTo, "assigned-to", "",
		"New assignee (use '-' to clear)",
	)
	trackUpdateCmd.Flags().StringVar(
		&trackUpdateType, "type", "",
		"New track type",
	)
	trackUpdateCmd.Flags().StringVar(
		&trackUpdateAddPlan, "add-plan", "",
		"Link a plan file and optionally extract tasks from its frontmatter",
	)
	trackUpdateCmd.Flags().StringVar(
		&trackUpdateDue, "due", "",
		"Due date (tomorrow, in 3d, 2025-05-01, RFC3339; use '-' to clear)",
	)
}
