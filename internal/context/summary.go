package contextstate

type Summary struct {
	Goal               string   `json:"goal"`
	Workspace          string   `json:"workspace"`
	Decisions          []string `json:"decisions"`
	CompletedWork      []string `json:"completed_work"`
	OpenThreads        []string `json:"open_threads"`
	ImportantArtifacts []string `json:"important_artifacts"`
	KnownConstraints   []string `json:"known_constraints"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type WorkingContext struct {
	RecentMessages []Message `json:"recent_messages"`
	Summaries      []Summary `json:"summaries"`
	MemoryRefs     []string  `json:"memory_refs"`
}
