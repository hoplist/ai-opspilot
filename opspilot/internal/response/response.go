package response

import (
	"time"

	"github.com/dualistpeng-netizen/ai-observability/opspilot/internal/version"
)

const BackendName = "opspilot-core"

type Envelope struct {
	OK       bool       `json:"ok"`
	Data     any        `json:"data,omitempty"`
	Error    *ErrorBody `json:"error,omitempty"`
	Warnings []string   `json:"warnings"`
	Source   SourceInfo `json:"source"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SourceInfo struct {
	Backend string `json:"backend"`
	Time    string `json:"time"`
	Version string `json:"version,omitempty"`
}

func OK(data any, warnings []string) Envelope {
	if warnings == nil {
		warnings = []string{}
	}
	return Envelope{
		OK:       true,
		Data:     data,
		Warnings: warnings,
		Source:   source(),
	}
}

func Error(code, message string) Envelope {
	return Envelope{
		OK: false,
		Error: &ErrorBody{
			Code:    code,
			Message: message,
		},
		Warnings: []string{},
		Source:   source(),
	}
}

func source() SourceInfo {
	return SourceInfo{
		Backend: BackendName,
		Time:    time.Now().Format(time.RFC3339),
		Version: version.Version,
	}
}
