package nodeagent

const (
	DefaultTailLines    = 300
	DefaultSinceSeconds = 1800
	DefaultLimitBytes   = 1024 * 1024
	MaxTailLines        = 1000
	MaxSinceSeconds     = 86400
	MaxLimitBytes       = 5 * 1024 * 1024
)

type LogRequest struct {
	Host         string
	Container    string
	TailLines    int
	SinceSeconds int
	LimitBytes   int
	Timestamps   bool
}

type ContainerLog struct {
	Host         string `json:"host,omitempty"`
	Container    string `json:"container"`
	TailLines    int    `json:"tail_lines"`
	SinceSeconds int    `json:"since_seconds"`
	LimitBytes   int    `json:"limit_bytes"`
	Truncated    bool   `json:"truncated"`
	Text         string `json:"text"`
}

func BoundedLogRequest(req LogRequest) LogRequest {
	req.TailLines = clamp(defaultInt(req.TailLines, DefaultTailLines), 1, MaxTailLines)
	req.SinceSeconds = clamp(defaultInt(req.SinceSeconds, DefaultSinceSeconds), 1, MaxSinceSeconds)
	req.LimitBytes = clamp(defaultInt(req.LimitBytes, DefaultLimitBytes), 1, MaxLimitBytes)
	return req
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
