package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

var trackUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a track",
	Args:  cobra.ExactArgs(1),
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

		if !titleChanged && !statusChanged && !assignedChanged &&
			!typeChanged && !addPlanChanged {
			return fmt.Errorf(
				"no update flags provided; use --title, --status, " +
					"--assigned-to, --type, or --add-plan",
			)
		}

		if typeChanged && !core.ValidTrackType(trackUpdateType) {
			return fmt.Errorf(
				"track type %q invalid; valid types: feature, bug, refactor",
				trackUpdateType,
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
			return nil
		})
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(w, "Updated track %s\n", id)

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

		if len(taskSpecs) > 0 {
			var projectID string
			if proj := core.DetectProject(); proj != nil &&
				proj.ProjectID != "" {
				projectID = proj.ProjectID
			}

			ids, createErr := svc.CreateTasksFromPlan(
				ctx, id, taskSpecs, projectID, s,
			)
			if createErr != nil {
				return fmt.Errorf(
					"plan linked but task creation failed: %w; "+
						"created %d of %d tasks",
					createErr, len(ids), len(taskSpecs),
				)
			}
			_, _ = fmt.Fprintf(
				w, "Linked plan %s, created %d tasks\n",
				trackUpdateAddPlan, len(ids),
			)
		} else if addPlanChanged && trackUpdateAddPlan != "" {
			_, _ = fmt.Fprintf(
				w, "Linked plan %s (no tasks extracted)\n",
				trackUpdateAddPlan,
			)
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
		"New track type (feature, bug, refactor)",
	)
	trackUpdateCmd.Flags().StringVar(
		&trackUpdateAddPlan, "add-plan", "",
		"Link a plan file and optionally extract tasks from its frontmatter",
	)
}
