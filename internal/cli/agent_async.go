package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
	"hop.top/tlc/internal/core"
)

// enqueueAgentJob serializes the current run flags into a job and enqueues it.
// Returns the job ID.
func enqueueAgentJob(cmd *cobra.Command) (string, error) {
	s, err := getStorage()
	if err != nil {
		return "", err
	}
	defer func() { _ = s.Close() }()

	// Determine job type.
	var jobType core.JobType
	switch {
	case len(agentRunTasks) > 0:
		jobType = core.JobTypeAgentTask
	case agentRunFlow != "":
		jobType = core.JobTypeAgentFlow
	case agentRunTrack != "":
		jobType = core.JobTypeAgentTrack
	}

	payload := &core.JobPayload{
		AgentName:        agentRunAgent,
		Tasks:            agentRunTasks,
		FlowRef:          agentRunFlow,
		TrackID:          agentRunTrack,
		Image:            agentRunImage,
		Local:            agentRunLocal,
		Prompt:           agentRunPrompt,
		NoState:          agentRunNoState,
		KeepPod:          agentRunKeepPod,
		Network:          agentRunNetwork,
		Retries:          agentRunRetries,
		TimeoutSecs:      int(agentRunTimeout.Seconds()),
		Env:              agentRunEnv,
		Mounts:           agentRunMounts,
		Context:          agentRunContext,
		TotalTimeoutSecs: int(agentRunTotalTimeout.Seconds()),
		TrustProject:     agentRunTrustProject,
	}

	payloadStr, err := core.MarshalPayload(payload)
	if err != nil {
		return "", err
	}

	job := &core.Job{
		ID:        uuid.New().String(),
		Queue:     "agent",
		Type:      jobType,
		Status:    core.JobStatusQueued,
		Payload:   payloadStr,
		CreatedAt: time.Now().UTC(),
		CreatedBy: core.GetCurrentUser(),
	}

	if err := s.CreateJob(context.Background(), job); err != nil {
		return "", fmt.Errorf("enqueue job: %w", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Job queued: %s\n", job.ID)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Type:  %s\n", job.Type)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Agent: %s\n", payload.AgentName)
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nRun 'tlc agent watch' to process, "+
		"or 'tlc agent status %s' to check.\n", job.ID)
	return job.ID, nil
}

// AgentStatusCmd implements `tlc agent status <job-id>`.
var AgentStatusCmd = &cobra.Command{
	Use:   "status <job-id>",
	Short: "Check async agent job status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		job, err := s.GetJob(context.Background(), jobID)
		if err != nil {
			return fmt.Errorf("get job: %w", err)
		}
		if job == nil {
			return fmt.Errorf("job %s not found", jobID)
		}

		format := viper.GetString("output.format")
		if format == formatJSON {
			return output.Render(cmd.OutOrStdout(), formatJSON, job)
		}

		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "Job: %s\n", job.ID)
		_, _ = fmt.Fprintf(out, "  Queue:   %s\n", job.Queue)
		_, _ = fmt.Fprintf(out, "  Type:    %s\n", job.Type)
		_, _ = fmt.Fprintf(out, "  Status:  %s\n", job.Status)
		// Job detail (table-style). Humanise the timestamps; JSON
		// branch above keeps RFC3339. T-1384.
		_, _ = fmt.Fprintf(out, "  Created: %s\n", DisplayTimeRelative(job.CreatedAt))
		if job.StartedAt != nil {
			_, _ = fmt.Fprintf(out, "  Started: %s\n", DisplayTimePtrRelative(job.StartedAt))
		}
		if job.EndedAt != nil {
			_, _ = fmt.Fprintf(out, "  Ended:   %s\n", DisplayTimePtrRelative(job.EndedAt))
		}
		if job.Error != "" {
			_, _ = fmt.Fprintf(out, "  Error:   %s\n", job.Error)
		}
		if job.Result != "" {
			_, _ = fmt.Fprintf(out, "  Result:  %s\n", job.Result)
		}
		return nil
	},
}

// AgentCancelCmd implements `tlc agent cancel <job-id>`.
var AgentCancelCmd = &cobra.Command{
	Use:   "cancel <job-id>",
	Short: "Cancel a queued agent job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jobID := args[0]
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		if err := s.CancelJob(context.Background(), jobID); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Canceled job %s\n", jobID)
		return nil
	},
}

// AgentWatchCmd implements `tlc agent watch` — polls the queue and executes.
var AgentWatchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Poll job queue and execute agent jobs",
	Long: `Long-running daemon that polls the agent job queue and
processes jobs as they arrive. Ctrl-C to stop.

Example:
  tlc agent watch`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		s, err := getStorage()
		if err != nil {
			return err
		}
		defer func() { _ = s.Close() }()

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Watching agent job queue...")

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			job, err := s.ClaimNextJob(ctx, "agent")
			if err != nil {
				_, _ = fmt.Fprintf(cmd.OutOrStderr(),
					"Error claiming job: %v\n", err)
				time.Sleep(5 * time.Second)
				continue
			}
			if job == nil {
				time.Sleep(2 * time.Second)
				continue
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"Processing job %s (type: %s)\n", job.ID, job.Type)

			if err := processJob(ctx, cmd, s, job); err != nil {
				_, _ = fmt.Fprintf(cmd.OutOrStderr(),
					"Job %s failed: %v\n", job.ID, err)
			}
		}
	},
}

func processJob(
	ctx context.Context,
	cmd *cobra.Command,
	s interface {
		core.JobStore
		core.Repository
		core.LogRepository
	},
	job *core.Job,
) error {
	payload, err := core.UnmarshalPayload(job.Payload)
	if err != nil {
		return markJobFailed(ctx, s, job, err)
	}

	// Wire up agent run flags from payload.
	agentRunAgent = payload.AgentName
	agentRunTasks = payload.Tasks
	agentRunFlow = payload.FlowRef
	agentRunTrack = payload.TrackID
	agentRunImage = payload.Image
	agentRunLocal = payload.Local
	agentRunPrompt = payload.Prompt
	agentRunNoState = payload.NoState
	agentRunKeepPod = payload.KeepPod
	agentRunNetwork = payload.Network
	agentRunRetries = payload.Retries
	agentRunEnv = payload.Env
	agentRunMounts = payload.Mounts
	agentRunContext = payload.Context
	agentRunTrustProject = payload.TrustProject
	agentRunAsync = false // execute synchronously
	agentRunDryRun = false
	if payload.TimeoutSecs > 0 {
		agentRunTimeout = time.Duration(payload.TimeoutSecs) * time.Second
	}
	if payload.TotalTimeoutSecs > 0 {
		agentRunTotalTimeout = time.Duration(payload.TotalTimeoutSecs) * time.Second
	}

	execErr := runAgentRun(cmd, nil)

	now := time.Now().UTC()
	job.EndedAt = &now

	if execErr != nil {
		job.Status = core.JobStatusFailed
		job.Error = execErr.Error()
	} else {
		job.Status = core.JobStatusSucceeded
	}

	if err := s.UpdateJob(ctx, job); err != nil {
		return fmt.Errorf("update job %s: %w", job.ID, err)
	}
	return execErr
}

func markJobFailed(ctx context.Context, s core.JobStore, job *core.Job, reason error) error {
	now := time.Now().UTC()
	job.Status = core.JobStatusFailed
	job.Error = reason.Error()
	job.EndedAt = &now
	return s.UpdateJob(ctx, job)
}

func init() {
	AgentCmd.AddCommand(AgentStatusCmd)
	AgentCmd.AddCommand(AgentCancelCmd)
	AgentCmd.AddCommand(AgentWatchCmd)
}
