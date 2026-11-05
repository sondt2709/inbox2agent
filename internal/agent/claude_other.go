//go:build !darwin

package agent

// readProc is not supported on this OS: the driver uses the Orca check only.
func readProc(int) (ProcInfo, error) { return ProcInfo{}, ErrClaudeUnknown }
