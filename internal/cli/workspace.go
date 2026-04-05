package cli

import (
	"encoding/json"
	"fmt"
	"os/exec"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/workspace"
)

// wsmDetected reports whether the wsm binary is available on PATH.
func wsmDetected() bool {
	_, err := exec.LookPath("wsm")
	return err == nil
}

var WorkspaceCmd = &cobra.Command{
	Use:     "workspace",
	Short:   "Manage workspaces",
	Aliases: []string{"ws"},
}

var WorkspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured workspaces and their projects",
	RunE:  runWorkspaceList,
}

// workspaceListData is the JSON-serializable representation of the output.
type workspaceListData struct {
	Name     string              `json:"name"`
	WsmID    string              `json:"wsm_id,omitempty"`
	Default  bool                `json:"default"`
	Spaces   []spaceListData     `json:"spaces"`
}

type spaceListData struct {
	URI      string                   `json:"uri"`
	Label    string                   `json:"label,omitempty"`
	Adapter  string                   `json:"adapter,omitempty"`
	Projects []core.RegisteredProject `json:"projects"`
}

func runWorkspaceList(cmd *cobra.Command, _ []string) error {
	var workspaces []config.WorkspaceConfig
	if err := viper.UnmarshalKey("workspaces", &workspaces); err != nil {
		return fmt.Errorf("failed to read workspace config: %w", err)
	}

	if len(workspaces) == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No workspaces configured")
		return nil
	}

	// Open global storage once for project discovery.
	store, err := getStorageRaw()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer func() { _ = store.Close() }()

	reg := workspace.NewRegistry()
	reg.Register(workspace.NewFilesystemAdapter(store))

	// Build data for all workspaces.
	var allData []workspaceListData
	for _, ws := range workspaces {
		wsData := workspaceListData{
			Name:    ws.Name,
			WsmID:   ws.WsmID,
			Default: ws.Default,
		}

		for _, sp := range ws.Spaces {
			spData := spaceListData{
				URI:     sp.URI,
				Label:   sp.Label,
				Adapter: sp.Adapter,
			}

			adapter, resolveErr := reg.Resolve(sp)
			if resolveErr == nil {
				projects, _ := adapter.Discover(sp)
				spData.Projects = projects
			}
			if spData.Projects == nil {
				spData.Projects = []core.RegisteredProject{}
			}

			wsData.Spaces = append(wsData.Spaces, spData)
		}

		allData = append(allData, wsData)
	}

	format := viper.GetString("output.format")
	if format == formatJSON {
		return printWorkspaceListJSON(cmd, allData)
	}
	printWorkspaceListTable(cmd, allData)
	return nil
}

func printWorkspaceListJSON(cmd *cobra.Command, data []workspaceListData) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
	return nil
}

func printWorkspaceListTable(cmd *cobra.Command, data []workspaceListData) {
	w := cmd.OutOrStdout()
	wsNameStyle := lipgloss.NewStyle().Foreground(primaryColor).Bold(true)
	spaceStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("43"))
	dimStyle := lipgloss.NewStyle().Foreground(mutedColor)

	for i, ws := range data {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}

		// Workspace header
		name := ws.Name
		if ws.Default {
			name += " *"
		}
		_, _ = fmt.Fprintf(w, "Workspace: %s\n", wsNameStyle.Render(name))

		if ws.WsmID != "" {
			_, _ = fmt.Fprintf(w, "  wsm_id: %s\n", ws.WsmID)
		}

		for _, sp := range ws.Spaces {
			_, _ = fmt.Fprintln(w)
			label := sp.Label
			if label == "" {
				label = sp.URI
			}
			_, _ = fmt.Fprintf(w, "  Space: %s (%s)\n",
				spaceStyle.Render(label), sp.URI)

			if len(sp.Projects) == 0 {
				_, _ = fmt.Fprintf(w, "    %s\n",
					dimStyle.Render("(no projects)"))
				continue
			}

			// Find max widths for alignment.
			maxID, maxPath := 0, 0
			for _, p := range sp.Projects {
				if len(p.ProjectID) > maxID {
					maxID = len(p.ProjectID)
				}
				if len(p.DBPath) > maxPath {
					maxPath = len(p.DBPath)
				}
			}

			for _, p := range sp.Projects {
				seen := p.LastSeenAt.Format("2006-01-02")
				_, _ = fmt.Fprintf(w, "    %-*s  %-*s  %-8s %s\n",
					maxID, p.ProjectID,
					maxPath, p.DBPath,
					p.Status,
					seen,
				)
			}
		}
	}
}

func init() {
	WorkspaceCmd.AddCommand(WorkspaceListCmd)
	if !wsmDetected() {
		WorkspaceCmd.Short = "Manage workspaces (requires wsm — not found in PATH)"
		WorkspaceCmd.Hidden = true
	}
	RootCmd.AddCommand(WorkspaceCmd)
}
