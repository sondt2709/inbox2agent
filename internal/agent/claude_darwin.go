package agent

import (
	"time"

	"golang.org/x/sys/unix"
)

// readProc reads the start time and the environment of a process of the
// same user. The kernel refuses kern.procargs2 for a process of another user.
func readProc(pid int) (ProcInfo, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid != int32(pid) {
		return ProcInfo{}, ErrNoProcess
	}
	start := time.Unix(int64(kp.Proc.P_starttime.Sec), int64(kp.Proc.P_starttime.Usec)*1000)
	b, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return ProcInfo{}, ErrNoProcess
	}
	return ProcInfo{Start: start, Handle: envValue(parseProcArgs(b), "ORCA_TERMINAL_HANDLE")}, nil
}
