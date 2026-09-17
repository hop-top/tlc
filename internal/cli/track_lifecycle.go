package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	kitcli "hop.top/kit/go/console/cli"
	"hop.top/tlc/internal/core"
)

var trackAbandonNoPrompt bool

var trackArchiveCmd = trackLifecycleCmd(
	"archive", "Archive a completed or abandoned track",
	`Mark a completed or abandoned track as archived.

Archived tracks are hidden from default listings but retained for
history. Re-running on an already-archived track converges.`,
	"destructive-local",
	func(ctx context.Context, svc *core.TrackService, id string) error {
		return checkTrackTransition(ctx, svc, id, core.TrackStatusArchived)
	},
	func(ctx context.Context, svc *core.TrackService, id string) error {
		return svc.ArchiveTrack(ctx, id)
	},
)

var trackAbandonCmd = &cobra.Command{
	Use:   "abandon <id>",
	Short: "Abandon a track and skip its non-terminal tasks",
	Long: `Abandon a track and skip every linked non-terminal task in a
single operation.

Prompts for confirmation when any non-terminal tasks would be skipped
unless --no-prompt is set. The track transitions to the abandoned
status and its tasks become SKIPPED.

--dry-run lists the track and every task that would be skipped, and
writes none of it.`,
	Annotations: map[string]string{
		"kit/side-effect": "destructive-local",
		"kit/idempotent":  "yes",
	},
	Args: cobra.ExactArgs(1),
	RunE: runTrackAbandon,
}

func init() {
	trackAbandonCmd.Flags().BoolVar(
		&trackAbandonNoPrompt, "no-prompt", false,
		"Skip confirmation prompt",
	)
}

func runTrackAbandon(cmd *cobra.Command, args []string) error {
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
	w := cmd.OutOrStdout()

	// Check for linked non-terminal tasks.
	affected, err := svc.LinkedNonTerminalTasks(ctx, id)
	if err != nil {
		return err
	}

	displayID := trackDisplayID(ctx, svc, id)

	if len(affected) > 0 && !trackAbandonNoPrompt {
		_, _ = fmt.Fprintf(w,
			"Abandoning track %s will skip %d task(s):\n",
			displayID, len(affected))
		for _, t := range affected {
			_, _ = fmt.Fprintf(w, "  %s  %s  [%s]\n",
				formatTaskAlias(t), t.Title, t.Status)
		}
		_, _ = fmt.Fprint(w, "Continue? [y/N] ")
		if !readConfirm(cmd.InOrStdin()) {
			return fmt.Errorf("aborted; track %s not abandoned", displayID)
		}
	}

	// Abandon is compound: AbandonTrackWithTasks skips every linked
	// non-terminal task and then moves the track. Stop ahead of it on a
	// dry run, but only after the confirmation above and the same
	// transition check it performs first — so the preview is refused by
	// exactly what would refuse the real run.
	if kitcli.IsDryRun(cmd) {
		if err := checkTrackTransition(
			ctx, svc, id, core.TrackStatusAbandoned,
		); err != nil {
			return err
		}
		printTrackAbandonDryRun(w, displayID, affected)
		return nil
	}

	skipped, err := svc.AbandonTrackWithTasks(ctx, id)
	if err != nil {
		return err
	}

	if len(skipped) > 0 {
		_, _ = fmt.Fprintf(w, "Skipped %d task(s)\n", len(skipped))
	}
	_, _ = fmt.Fprintf(w, "Abandoned track %s\n", displayID)
	return nil
}

// printTrackAbandonDryRun reports the whole blast radius of an abandon:
// the track and every linked task that would be skipped with it.
//
// Naming the tasks is the point. An abandon is not one write but N+1,
// and SKIPPED is terminal, so a preview that reported only the track
// would understate exactly the part a user cannot undo.
func printTrackAbandonDryRun(w io.Writer, displayID string, affected []*core.Task) {
	_, _ = fmt.Fprintf(w, "Dry run — would abandon track %s\n", displayID)
	if len(affected) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "  Would skip %d task(s):\n", len(affected))
	for _, t := range affected {
		_, _ = fmt.Fprintf(w, "    %s  %s  [%s]\n",
			formatTaskAlias(t), t.Title, t.Status)
	}
}

// trackDisplayID returns the human-readable display alias (slug) for a
// track when available, falling back to the durable typeid. Errors during
// lookup degrade to the typeid.
func trackDisplayID(ctx context.Context, svc *core.TrackService, id string) string {
	if svc == nil || id == "" {
		return id
	}
	track, err := svc.GetTrack(ctx, id)
	if err != nil || track == nil {
		return id
	}
	return formatTrackAlias(track)
}

var trackDeleteCmd = trackLifecycleCmd(
	"delete", "Delete a track (fails if tasks are linked)",
	`Delete a track from the local store.

Fails when any task is still linked to the track; clear the links
first or use 'tlc track abandon' to skip them. The deletion is local
and irreversible.`,
	"destructive-local",
	checkTrackDeletable,
	func(ctx context.Context, svc *core.TrackService, id string) error {
		return svc.DeleteTrack(ctx, id)
	},
)

// trackLifecycleCmd builds a cobra.Command that resolves a track ID, creates a
// TrackService, and delegates to action. The verb is used in both the Use line
// and the confirmation message.
//
// --dry-run is honored here, in the factory, rather than inside each
// action closure. The factory owns the whole write path — resolve,
// service, action, success line — so this is the one place that covers
// every verb it builds, including any added later. A guard per closure
// would leave the next caller of this factory silently writing under
// --dry-run again, which is the defect being fixed.
//
// Each verb supplies a preflight that re-runs, read-only, whatever
// would refuse the real thing — the state machine for archive, the
// linked-task rule for delete. The preview reports only once that
// passes, so it is refused by exactly what would refuse the write
// rather than promising an operation that cannot happen.
func trackLifecycleCmd(
	verb, short, long, sideEffect string,
	preflight func(ctx context.Context, svc *core.TrackService, id string) error,
	action func(ctx context.Context, svc *core.TrackService, id string) error,
) *cobra.Command {
	// Capitalise first letter for the output message.
	past := strings.ToUpper(verb[:1]) + verb[1:]
	return &cobra.Command{
		Use:   verb + " <id>",
		Short: short,
		Long:  long,
		Annotations: map[string]string{
			"kit/side-effect": sideEffect,
			"kit/idempotent":  "yes",
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
			// Resolve display alias before the action runs since some
			// destructive verbs (e.g. delete) remove the row first.
			displayID := trackDisplayID(ctx, svc, id)

			w := cmd.OutOrStdout()

			// The track resolved and the caller cleared the
			// confirmation gate. Run the verb's own refusal checks,
			// then stop: the action is the only thing left that
			// writes.
			if kitcli.IsDryRun(cmd) {
				if preflight != nil {
					if err := preflight(ctx, svc, id); err != nil {
						return err
					}
				}
				_, _ = fmt.Fprintf(w, "Dry run — would %s track %s\n", verb, displayID)
				return nil
			}

			if err := action(ctx, svc, id); err != nil {
				return err
			}

			_, _ = fmt.Fprintf(w, "%sd track %s\n", past, displayID)
			return nil
		},
	}
}

// checkTrackTransition runs, read-only, the state-machine check the
// write path would hit when moving a track to `to`.
//
// The linked-task inputs come from GetTrackWithState: TotalTasks is the
// linked count and CompletedTasks counts terminal statuses through the
// same workflow predicate TrackService.linkedTaskStats uses, so
// "all terminal" here means what it means on the write path.
func checkTrackTransition(
	ctx context.Context, svc *core.TrackService, id string, to core.TrackStatus,
) error {
	track, _, progress, err := svc.GetTrackWithState(ctx, id, 0)
	if err != nil {
		return err //nolint:wrapcheck // the service names the track it could not read
	}
	allTerminal := progress.TotalTasks == progress.CompletedTasks
	if err := core.ValidateTrackTransition(
		track.Status, to, progress.TotalTasks, allTerminal,
	); err != nil {
		return err //nolint:wrapcheck // the error names both statuses and the allowed set
	}
	return nil
}

// checkTrackDeletable mirrors the linked-task refusal the delete
// transaction enforces in storage. Deleting is not a transition, so the
// state machine has nothing to say about it; what refuses a real delete
// is a track that still owns tasks.
//
// The message is worded as the storage one is, so a preview and a real
// run are refused in the same terms.
func checkTrackDeletable(
	ctx context.Context, svc *core.TrackService, id string,
) error {
	_, _, progress, err := svc.GetTrackWithState(ctx, id, 0)
	if err != nil {
		return err //nolint:wrapcheck // the service names the track it could not read
	}
	if progress.TotalTasks > 0 {
		return fmt.Errorf(
			"track %q has %d linked task(s); unlink them first with "+
				"'tlc task update <id> --track -'",
			id, progress.TotalTasks,
		)
	}
	return nil
}
