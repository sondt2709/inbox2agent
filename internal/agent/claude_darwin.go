package agent

import (
	"time"

	"golang.org/x/sys/unix"
)

// readProc reads the start time and the environment of a process of the
// same user: its Orca terminal and whether Orca started it as
// claude-agent-teams. The kernel refuses kern.procargs2 for a process of
// another user.
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
	env := parseProcArgs(b)
	return ProcInfo{Start: start, Handle: envValue(env, "ORCA_TERMINAL_HANDLE"), Teams: envValue(env, "ORCA_AGENT_TEAMS_TEAM_ID") != ""}, nil
}
