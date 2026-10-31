package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ErrBusy is returned when another process sends to the agent of the space
// for longer than the context allows.
var ErrBusy = errors.New("another delivery to the agent of the space is running")

// lockRetry is the time between two tries of Lock.
const lockRetry = 100 * time.Millisecond

// Lock takes the delivery lock of a space: an exclusive flock on
// <dir>/space-<id>.lock. A terminal belongs to one space. The daemon and an ask hold it while they type
// into an agent terminal, so two prompts never mix in one input box. Lock
// tries again until ctx ends. Call the returned function to release the lock.
func Lock(ctx context.Context, dir, space string) (func(), error) {
	path := filepath.Join(dir, "space-"+space+".lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("lock %s: %w", path, err)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ErrBusy
		case <-time.After(lockRetry):
		}
	}
}
