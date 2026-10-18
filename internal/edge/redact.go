package edge

import (
	"regexp"
	"strings"

	"prisme.local/platform/internal/observability"
)

var sensitiveLine = regexp.MustCompile(`(?im)^.*\b(?:authorization|cookie|password|passwd|token|secret|api[_-]?key)\b.*$`)

func publicSource(s string) string {
	return sensitiveLine.ReplaceAllString(observability.Redact(s), "# [REDACTED]")
}
func publicConfig(v any) any {
	switch value := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range value {
			lower := strings.ToLower(k)
			if strings.Contains(lower, "password") || strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "cookie") || lower == "authorization" || lower == "api_key" || lower == "key" {
				out[k] = "[REDACTED]"
			} else {
				out[k] = publicConfig(v)
			}
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, v := range value {
			out[i] = publicConfig(v)
		}
		return out
	case string:
		return observability.Redact(value)
	default:
		return value
	}
}
