package tools

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// getArgs safely extracts arguments from the MCP request.
// In mcp-go v0.32+, Arguments is `any` type, so we need type assertion.
func getArgs(request mcp.CallToolRequest) map[string]interface{} {
	if args, ok := request.Params.Arguments.(map[string]interface{}); ok {
		return normalizeArgs(args)
	}
	return make(map[string]interface{})
}

// normalizeArgs applies lightweight normalization to make tools more tolerant
// of common model/client argument shape variations.
func normalizeArgs(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return make(map[string]interface{})
	}

	args := make(map[string]interface{}, len(raw)+8)
	for k, v := range raw {
		switch typed := v.(type) {
		case string:
			args[k] = strings.TrimSpace(typed)
		case float64:
			// ID-like fields are often sent as numbers by models/clients.
			if strings.HasSuffix(k, "_id") {
				args[k] = fmt.Sprintf("%.0f", typed)
			} else {
				args[k] = typed
			}
		case int, int32, int64:
			if strings.HasSuffix(k, "_id") {
				args[k] = fmt.Sprintf("%v", typed)
			} else {
				args[k] = typed
			}
		default:
			args[k] = v
		}
	}

	setAlias := func(target string, sources ...string) {
		if existing := getFirstStringArg(args, target); existing != "" {
			return
		}
		if resolved := getFirstStringArg(args, sources...); resolved != "" {
			args[target] = resolved
		}
	}

	setAlias("conversation_id", "conversation_id", "channel_id", "group_id")
	setAlias("group_id", "group_id", "conversation_id")
	setAlias("path", "path", "file_path")
	setAlias("before_id", "before_id", "before")

	return args
}

// getStringArg extracts a string argument with validation.
func getStringArg(args map[string]interface{}, key string, required bool) (string, error) {
	v, ok := args[key].(string)
	if required && (!ok || strings.TrimSpace(v) == "") {
		return "", fmt.Errorf("%s is required", key)
	}
	return strings.TrimSpace(v), nil
}

// getFirstStringArg returns the first non-empty string from the provided keys.
func getFirstStringArg(args map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := args[key].(string); ok {
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

// getIntArg extracts an integer argument with default value and bounds checking.
func getIntArg(args map[string]interface{}, key string, defaultVal, minVal, maxVal int) int {
	if v, ok := args[key].(float64); ok {
		val := int(v)
		if val < minVal {
			return minVal
		}
		if maxVal > 0 && val > maxVal {
			return maxVal
		}
		return val
	}
	return defaultVal
}

// getBoolArg extracts a boolean argument with default value.
func getBoolArg(args map[string]interface{}, key string, defaultVal bool) bool {
	if v, ok := args[key].(bool); ok {
		return v
	}
	return defaultVal
}
