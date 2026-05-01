package router

// Route returns "local" or "passthrough" for the given model name.
func Route(model, localName string) string {
	if model == localName {
		return "local"
	}
	return "passthrough"
}
