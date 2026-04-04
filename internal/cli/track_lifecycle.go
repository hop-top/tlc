package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"hop.top/tlc/internal/core"
)

var trackArchiveCmd = &cobra.Command{
	Use:   "archive <id>",
	Short: "Archive a completed or abandoned track",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		s, err := getStorageRaw()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		svc := core.NewTrackService(s, s)
		ctx := context.Background()

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
	Short: "Abandon an active track",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		s, err := getStorageRaw()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		svc := core.NewTrackService(s, s)
		ctx := context.Background()

		if err := svc.AbandonTrack(ctx, id); err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(w, "Abandoned track %s\n", id)
		return nil
	},
}

var trackDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a track (fails if tasks are linked)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		s, err := getStorageRaw()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		svc := core.NewTrackService(s, s)
		ctx := context.Background()

		if err := svc.DeleteTrack(ctx, id); err != nil {
			return err
		}

		w := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(w, "Deleted track %s\n", id)
		return nil
	},
}
