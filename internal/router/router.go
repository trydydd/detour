package router

import "strings"

// Route returns "local" or "passthrough" for the given model name.
func Route(model, localName string) string {
	if model == localName {
		return "local"
	}
	if strings.Contains(strings.ToLower(model), "opus") {
		return "passthrough"
	}
	return "local"
}
