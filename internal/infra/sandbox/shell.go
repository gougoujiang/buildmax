package sandbox

// shellOrDefault returns shell when non-empty, else /bin/sh. Shared by
// every backend so the choice of inner shell is consistent across
// platforms when the caller leaves it empty.
func shellOrDefault(shell string) string {
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}
