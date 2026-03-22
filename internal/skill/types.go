package skill

type Source string

const (
	SourceWorkspace Source = "workspace"
	SourceUser      Source = "user"
	SourceBundled   Source = "bundled"
)

type Definition struct {
	Name                    string   `json:"name"`
	Description             string   `json:"description"`
	UserInvocable           bool     `json:"user_invocable"`
	Source                  Source   `json:"source"`
	FilePath                string   `json:"file_path"`
	RuntimePath             string   `json:"runtime_path,omitempty"`
	RequiresPlugin          string   `json:"requires_plugin,omitempty"`
	RequiresBins            []string `json:"requires_bins,omitempty"`
	RequiresEnv             []string `json:"requires_env,omitempty"`
	OS                      []string `json:"os,omitempty"`
	Arch                    []string `json:"arch,omitempty"`
	RecommendedTools        []string `json:"recommended_tools,omitempty"`
	RecommendedProjectFiles []string `json:"recommended_project_files,omitempty"`
	WakePhases              []string `json:"wake_phases,omitempty"`
	Content                 string   `json:"content,omitempty"`
}

type Diagnostic struct {
	Path    string `json:"path,omitempty"`
	Message string `json:"message"`
}

type Snapshot struct {
	Version     int64        `json:"version"`
	Skills      []Definition `json:"skills"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type SourceDir struct {
	Source Source
	Dir    string
}

type AvailabilityOptions struct {
	OS               string
	Arch             string
	InstalledPlugins []string
	HasEnv           func(string) bool
	HasCommand       func(string) bool
}

type LoadOptions struct {
	Sources      []SourceDir
	Availability AvailabilityOptions
}
