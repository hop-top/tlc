package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/kit/go/core/util"
	"hop.top/tlc/internal/core"
)

var trackUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a track",
	Long: `Update a track's title, status, assignee, type, due date, or append
a plan file.

Status transitions follow the TrackService state machine. --add-plan
attaches an additional planning document to the track's plan directory.

--dry-run reports what would change — including the tasks --add-plan
would create or reconcile — and writes nothing. The preview is refused
by the same validation and state machine as a real update.`,
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

		// Named once and shared by the write path and the dry-run
		// preview below, so the preview cannot describe a different
		// mutation from the one a real run would apply.
		applyUpdates := func(t *core.Track) error {
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
		}

		// Everything that can refuse this update up front has now run:
		// the flag set, the track type, the plan's frontmatter and its
		// track references, and --due. The state machine is checked
		// inside the preview, against the same mutation, so a dry run
		// is refused by exactly what would refuse the real one.
		//
		// The preview must also cover the plan branch below: --add-plan
		// creates or reconciles tasks, which is a far larger write than
		// the track row itself.
		if kitcli.IsDryRun(cmd) {
			return trackUpdateDryRun(ctx, w, svc, id, applyUpdates, planTaskSpecs{
				addPlan: addPlanChanged && trackUpdateAddPlan != "",
				path:    trackUpdateAddPlan,
				fm:      planFm,
			})
		}

		if err := svc.UpdateTrack(ctx, id, applyUpdates); err != nil {
			return err
		}

		displayID := trackDisplayID(ctx, svc, id)
		_, _ = fmt.Fprintf(w, "Updated track %s\n", displayID)

		// Resolve task specs: frontmatter first, then extractor fallback.
		taskSpecs := resolvePlanTaskSpecs(w, planTaskSpecs{
			addPlan: addPlanChanged && trackUpdateAddPlan != "",
			path:    trackUpdateAddPlan,
			fm:      planFm,
		})

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

// planTaskSpecs names the --add-plan inputs the task-ingest branch
// works from, so the write path and the dry-run preview resolve specs
// through one code path instead of two that can drift.
type planTaskSpecs struct {
	addPlan bool
	path    string
	fm      *core.PlanFrontmatter
}

// resolvePlanTaskSpecs returns the tasks a plan would contribute:
// frontmatter when it declares any, otherwise the configured extractor.
// An extractor failure is reported and treated as "no specs", as the
// write path has always done.
func resolvePlanTaskSpecs(w io.Writer, p planTaskSpecs) []core.PlanTaskSpec {
	if p.fm != nil && len(p.fm.Tasks) > 0 {
		return p.fm.Tasks
	}
	if !p.addPlan {
		return nil
	}
	extCmd := viper.GetString("tracks.plan_extractor")
	if extCmd == "" {
		return nil
	}
	extracted, extErr := core.RunPlanExtractor(extCmd, p.path)
	if extErr != nil {
		_, _ = fmt.Fprintf(w, "Warning: extractor failed: %v\n", extErr)
		return nil
	}
	return extracted
}

// trackUpdateDryRun previews a `track update` without writing anything:
// no track row, no plan link, no tasks created or reconciled from the
// plan.
//
// The mutation is applied to a COPY of the stored track so the state
// machine can be checked against the result the real run would produce.
// That check is the reason the preview reads the track at all: without
// it, --dry-run would report a would-update for a transition
// UpdateTrack refuses, and the flag would become a way around the
// rules rather than a preview of them.
func trackUpdateDryRun(
	ctx context.Context,
	w io.Writer,
	svc *core.TrackService,
	id string,
	apply func(*core.Track) error,
	plan planTaskSpecs,
) error {
	track, _, progress, err := svc.GetTrackWithState(ctx, id, 0)
	if err != nil {
		return err //nolint:wrapcheck // the service names the track it could not read
	}

	before := track.Status
	preview := *track
	if err := apply(&preview); err != nil {
		return err
	}

	if preview.Status != before {
		allTerminal := progress.TotalTasks == progress.CompletedTasks
		if err := core.ValidateTrackTransition(
			before, preview.Status, progress.TotalTasks, allTerminal,
		); err != nil {
			return err //nolint:wrapcheck // the error names both statuses and the allowed set
		}
	}

	displayID := formatTrackAlias(track)
	_, _ = fmt.Fprintf(w, "Dry run — would update track %s\n", displayID)
	if preview.Title != track.Title {
		_, _ = fmt.Fprintf(w, "  Title:  %s\n", preview.Title)
	}
	if preview.Status != before {
		_, _ = fmt.Fprintf(w, "  Status: %s → %s\n", before, preview.Status)
	}
	if preview.Type != track.Type {
		_, _ = fmt.Fprintf(w, "  Type:   %s\n", preview.Type)
	}
	if preview.AssignedTo == nil {
		if track.AssignedTo != nil {
			_, _ = fmt.Fprint(w, "  Assigned: (cleared)\n")
		}
	} else if track.AssignedTo == nil || *preview.AssignedTo != *track.AssignedTo {
		_, _ = fmt.Fprintf(w, "  Assigned: %s\n", *preview.AssignedTo)
	}

	if !plan.addPlan {
		return nil
	}

	// The plan branch is the larger half of this command's blast
	// radius: it creates or reconciles one task per spec. Name them
	// rather than reporting a count, so the preview says what would
	// land and under which heading.
	_, _ = fmt.Fprintf(w, "  Plan:   %s\n", plan.path)
	specs := resolvePlanTaskSpecs(w, plan)
	if len(specs) == 0 {
		_, _ = fmt.Fprint(w, "  Would extract no tasks from the plan\n")
		return nil
	}
	verb := "create"
	if len(track.PlanMapping) > 0 {
		// A track that already carries a mapping takes the
		// reconciliation path on a real run, not a first ingest.
		verb = "reconcile"
	}
	_, _ = fmt.Fprintf(w, "  Would %s %d task(s) from the plan:\n", verb, len(specs))
	for _, spec := range specs {
		_, _ = fmt.Fprintf(w, "    %s\n", spec.Title)
	}
	return nil
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
		"New status",
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
