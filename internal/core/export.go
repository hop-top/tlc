package core

// ProjectExport represents an exportable snapshot of a project.
type ProjectExport struct {
	Version   string      `json:"version" yaml:"version"`
	ProjectID string      `json:"project_id" yaml:"project_id"`
	Tasks     []*Task     `json:"tasks" yaml:"tasks"`
	Logs      []*LogEntry `json:"logs,omitempty" yaml:"logs,omitempty"`
}
