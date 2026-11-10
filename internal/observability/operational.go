package observability

func isOperationalPath(path string) bool {
	switch path {
	case "/live", "/ready", "/health", "/metrics":
		return true
	default:
		return false
	}
}
