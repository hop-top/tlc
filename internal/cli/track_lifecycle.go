package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var trackAbandonNoPrompt bool

var trackArchiveCmd = &cobra.Command{
	Use:   "archive <id>",
	Short: "Archive a completed or abandoned track",
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
		if err := svc.ArchiveTrack(ctx, id); err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(w, "Archived track %s\n", id)
		return nil
	},
}

var trackAbandonCmd = &cobra.Command{
	Use:   "abandon <id>",
	Short: "Abandon a track and skip its non-terminal tasks",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrackAbandon,
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

	if len(affected) > 0 && !trackAbandonNoPrompt {
		_, _ = fmt.Fprintf(w,
			"Abandoning track %s will skip %d task(s):\n",
			id, len(affected))
		for _, t := range affected {
			_, _ = fmt.Fprintf(w, "  %s  %s  [%s]\n",
				t.ID, t.Title, t.Status)
		}
		_, _ = fmt.Fprint(w, "Continue? [y/N] ")
		if !readConfirm(os.Stdin) {
			return fmt.Errorf("aborted; track %s not abandoned", id)
		}
	}

	skipped, err := svc.AbandonTrackWithTasks(ctx, id)
	if err != nil {
		return err
	}

	if len(skipped) > 0 {
		_, _ = fmt.Fprintf(w, "Skipped %d task(s)\n", len(skipped))
	}
	_, _ = fmt.Fprintf(w, "Abandoned track %s\n", id)
	return nil
}

var trackDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a track (fails if tasks are linked)",
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
		if err := svc.DeleteTrack(ctx, id); err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(w, "Deleted track %s\n", id)
		return nil
	},
}
