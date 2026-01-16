package labels

// ProjectType represents a categorized project type
type ProjectType string

const (
	TypeGoBinary      ProjectType = "go-binary"
	TypeGoSocket      ProjectType = "go-socket"
	TypePythonMVC     ProjectType = "python-mvc"
	TypeReactFrontend ProjectType = "react-frontend"
	TypeMicroservices  ProjectType = "microservices"
	TypeGeneric       ProjectType = "generic"
)

// Label represents a GitHub label
type Label struct {
	Name        string
	Color       string
	Description string
}

// GetTemplates returns suggested labels for a project type
func GetTemplates(projectType ProjectType) []Label {
	common := []Label{
		{Name: "feat", Color: "0052CC", Description: "New feature"},
		{Name: "fix", Color: "D73A4A", Description: "Bug fix"},
		{Name: "priority:critical", Color: "B60205", Description: "Critical priority"},
		{Name: "priority:high", Color: "D93F0B", Description: "High priority"},
		{Name: "priority:medium", Color: "FBCA04", Description: "Medium priority"},
		{Name: "priority:low", Color: "0E8A16", Description: "Low priority"},
	}

	var domains []Label
	switch projectType {
	case TypeGoBinary:
		domains = []Label{
			{Name: "domain:cli", Color: "BFD4F2", Description: "CLI"},
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:config", Color: "0E8A16", Description: "Config"},
			{Name: "domain:io", Color: "1D76DB", Description: "I/O"},
		}
	case TypeReactFrontend:
		domains = []Label{
			{Name: "domain:frontend", Color: "E99695", Description: "Frontend UI"},
			{Name: "domain:components", Color: "1D76DB", Description: "Components"},
			{Name: "domain:hooks", Color: "0052CC", Description: "Hooks"},
		}
	default:
		domains = []Label{
			{Name: "domain:core", Color: "0052CC", Description: "Core logic"},
			{Name: "domain:api", Color: "1D76DB", Description: "API layer"},
		}
	}

	return append(common, domains...)
}
