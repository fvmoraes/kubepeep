package dashboard

import "time"

const (
	DefaultPageSize       = 100
	DefaultMaxItems       = 5_000
	DefaultMaxPages       = 50
	DefaultRestartLimit   = 10
	MaximumRestartLimit   = 50
	DefaultLogTailLines   = 200
	MaximumLogTailLines   = 2_000
	DefaultLogMaxPods     = 20
	MaximumLogMaxPods     = 50
	DefaultLogConcurrency = 4
	MaximumLogConcurrency = 8
	DefaultMaxContainers  = 100
	MaximumMaxContainers  = 400
	MaximumLogLineBytes   = 64 << 10
	MaximumContainerBytes = 2 << 20
	MaximumLogScanBytes   = 10 << 20
	MaximumExcerptBytes   = 4 << 10
	MaximumProblemText    = 4 << 10
	MaximumStatusBytes    = 256
	DefaultBlockTimeout   = 8 * time.Second
	DefaultLogWindow      = 15 * time.Minute
	MaximumLogWindow      = 4 * time.Hour
)

// QueryBudget bounds a Kubernetes list fan-out independently for every block.
type QueryBudget struct {
	Timeout  time.Duration
	PageSize int
	MaxItems int
	MaxPages int
}

func DefaultQueryBudget() QueryBudget {
	return QueryBudget{
		Timeout:  DefaultBlockTimeout,
		PageSize: DefaultPageSize,
		MaxItems: DefaultMaxItems,
		MaxPages: DefaultMaxPages,
	}
}

func (b QueryBudget) normalized() QueryBudget {
	defaults := DefaultQueryBudget()
	if b.Timeout <= 0 || b.Timeout > DefaultBlockTimeout {
		b.Timeout = defaults.Timeout
	}
	if b.PageSize <= 0 || b.PageSize > 500 {
		b.PageSize = defaults.PageSize
	}
	if b.MaxItems <= 0 {
		b.MaxItems = defaults.MaxItems
	}
	if b.MaxPages <= 0 {
		b.MaxPages = defaults.MaxPages
	}
	return b
}

// LogBudget is not user configurable. Request values may only reduce these
// hard server-side ceilings.
type LogBudget struct {
	Timeout           time.Duration
	MaxLineBytes      int64
	MaxContainerBytes int64
	MaxScanBytes      int64
	MaxContainers     int
}

func DefaultLogBudget() LogBudget {
	return LogBudget{
		Timeout:           DefaultBlockTimeout,
		MaxLineBytes:      MaximumLogLineBytes,
		MaxContainerBytes: MaximumContainerBytes,
		MaxScanBytes:      MaximumLogScanBytes,
		MaxContainers:     DefaultMaxContainers,
	}
}

func (b LogBudget) normalized() LogBudget {
	d := DefaultLogBudget()
	if b.Timeout <= 0 || b.Timeout > DefaultBlockTimeout {
		b.Timeout = d.Timeout
	}
	if b.MaxLineBytes <= 0 || b.MaxLineBytes > MaximumLogLineBytes {
		b.MaxLineBytes = d.MaxLineBytes
	}
	if b.MaxContainerBytes <= 0 || b.MaxContainerBytes > MaximumContainerBytes {
		b.MaxContainerBytes = d.MaxContainerBytes
	}
	if b.MaxScanBytes <= 0 || b.MaxScanBytes > MaximumLogScanBytes {
		b.MaxScanBytes = d.MaxScanBytes
	}
	if b.MaxContainers <= 0 || b.MaxContainers > MaximumMaxContainers {
		b.MaxContainers = d.MaxContainers
	}
	return b
}

type LogScanRequest struct {
	Window                  *string `json:"window,omitempty"`
	TailLines               *int    `json:"tailLines,omitempty"`
	MaxPods                 *int    `json:"maxPods,omitempty"`
	MaxConcurrentContainers *int    `json:"maxConcurrentContainers,omitempty"`
}

type ResolvedLogScanRequest struct {
	Window                  time.Duration
	TailLines               int
	MaxPods                 int
	MaxConcurrentContainers int
}

// ResolveLogScanRequest validates without silently clamping values.
func ResolveLogScanRequest(request LogScanRequest) (ResolvedLogScanRequest, error) {
	resolved := ResolvedLogScanRequest{
		Window:                  DefaultLogWindow,
		TailLines:               DefaultLogTailLines,
		MaxPods:                 DefaultLogMaxPods,
		MaxConcurrentContainers: DefaultLogConcurrency,
	}
	if request.Window != nil {
		windows := map[string]time.Duration{
			"15m": 15 * time.Minute,
			"30m": 30 * time.Minute,
			"1h":  time.Hour,
			"4h":  4 * time.Hour,
		}
		value, ok := windows[*request.Window]
		if !ok {
			return ResolvedLogScanRequest{}, validationError("window must be one of 15m, 30m, 1h, or 4h")
		}
		resolved.Window = value
	}
	if request.TailLines != nil {
		if *request.TailLines < 1 || *request.TailLines > MaximumLogTailLines {
			return ResolvedLogScanRequest{}, validationError("tailLines must be between 1 and 2000")
		}
		resolved.TailLines = *request.TailLines
	}
	if request.MaxPods != nil {
		if *request.MaxPods < 1 || *request.MaxPods > MaximumLogMaxPods {
			return ResolvedLogScanRequest{}, validationError("maxPods must be between 1 and 50")
		}
		resolved.MaxPods = *request.MaxPods
	}
	if request.MaxConcurrentContainers != nil {
		if *request.MaxConcurrentContainers < 1 || *request.MaxConcurrentContainers > MaximumLogConcurrency {
			return ResolvedLogScanRequest{}, validationError("maxConcurrentContainers must be between 1 and 8")
		}
		resolved.MaxConcurrentContainers = *request.MaxConcurrentContainers
	}
	return resolved, nil
}
