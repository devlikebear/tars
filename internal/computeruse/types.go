package computeruse

// Element is one node of a window's accessibility tree, flattened into the
// snapshot the model reads.
type Element struct {
	Index    int    // 1-based, 스냅샷 내 순번. state에서 eN으로 표기
	Token    string // cua-driver element_token
	Role     string // "AXButton" 등 원본 role 문자열
	Label    string
	Value    string
	Enabled  bool
	Selected *bool // nil이면 해당 없음
	Secure   bool  // secure text field → value 절대 노출 안 함
}

// Window identifies the GUI window a step observes and acts on.
type Window struct {
	PID      int
	WindowID int
	App      string
	Title    string
}

// Snapshot is one observation of a window: the elements the driver returned,
// how many it saw in total, and why the view is degraded, if it is.
type Snapshot struct {
	Window        Window
	Elements      []Element
	TotalElements int
	Degraded      string
}

// SnapshotOpts narrows what the driver collects for a snapshot.
type SnapshotOpts struct {
	Query    string
	MaxDepth int
}

// Status is the terminal outcome of a run.
type Status string

const (
	StatusDone              Status = "done"
	StatusStuck             Status = "stuck"
	StatusNeedsConfirmation Status = "needs_confirmation"
	StatusCancelled         Status = "cancelled"
	StatusMaxSteps          Status = "max_steps"
	StatusUnavailable       Status = "unavailable"
	StatusError             Status = "error"
)

// Request is a single plain-language goal to drive a GUI toward.
type Request struct {
	Goal     string
	App      string
	Inputs   map[string]string
	MaxSteps int
}

// TraceStep records one observe-decide-act cycle.
type TraceStep struct {
	Step        int     `json:"step"`
	Op          string  `json:"op"`
	Target      string  `json:"target,omitempty"`
	InputKey    string  `json:"input_key,omitempty"`
	Confidence  float64 `json:"confidence"`
	Risky       float64 `json:"risky"`
	Done        float64 `json:"done"`
	Effect      string  `json:"effect"`
	LatencyMS   int64   `json:"latency_ms"`
	InputTokens int     `json:"input_tokens"`
	Note        string  `json:"note,omitempty"`
}

// ProposedAction is the action a run stopped short of taking, waiting for the
// caller to confirm it.
type ProposedAction struct {
	Op       string  `json:"op"`
	Target   string  `json:"target"`
	InputKey string  `json:"input_key,omitempty"`
	Risky    float64 `json:"risky"`
}

// Usage is what the run cost.
type Usage struct {
	JevInputTokens int     `json:"jev_input_tokens"`
	EstUSD         float64 `json:"est_usd"`
	ElapsedMS      int64   `json:"elapsed_ms"`
}

// Result is the full outcome of a run, serialized verbatim to the caller.
type Result struct {
	Status         Status          `json:"status"`
	Reason         string          `json:"reason,omitempty"`
	Hint           string          `json:"hint,omitempty"`
	Steps          int             `json:"steps"`
	Trace          []TraceStep     `json:"trace"`
	ProposedAction *ProposedAction `json:"proposed_action,omitempty"`
	Resume         string          `json:"resume,omitempty"`
	LastScreen     []string        `json:"last_screen,omitempty"`
	Usage          Usage           `json:"usage"`
}
