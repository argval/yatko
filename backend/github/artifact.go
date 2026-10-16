package github

// ArtifactTarget keeps OS, CPU, and runtime requirements together for one build.
type ArtifactTarget struct {
	Platform string `json:"platform"`
	Arch     string `json:"arch,omitempty"`
	Libc     string `json:"libc,omitempty"`
}

// ArtifactEvidence identifies the source of a fact without inventing asset URLs.
type ArtifactEvidence struct {
	Source string `json:"source"`
	Field  string `json:"field"`
	Value  string `json:"value"`
}

// ArtifactMetadata holds observed facts; absent targets remain unknown.
type ArtifactMetadata struct {
	Targets   []ArtifactTarget   `json:"targets"`
	Role      string             `json:"role"`
	Evidence  []ArtifactEvidence `json:"evidence"`
	Status    string             `json:"status"`
	Conflicts []string           `json:"conflicts,omitempty"`
}
