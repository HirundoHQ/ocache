package ocache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/olric-data/olric"
)

// ErrLockNotAcquired is returned by WithLock when someone else still holds
// the lock after the wait elapses.
var ErrLockNotAcquired = errors.New("lock not acquired")

// isLockNotAcquired mirrors isKeyNotFound's plain-string fallback.
func isLockNotAcquired(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, olric.ErrLockNotAcquired) || err.Error() == olric.ErrLockNotAcquired.Error()
}

// lockSlice bounds one LockWithTimeout round trip: the Olric client waits
// for a single reply under its 3 s read timeout, so longer waits are sliced.
const lockSlice = time.Second

// WithLock runs fn under a cluster-wide lock on key that expires after ttl
// (at least 1 ms, Olric's granularity) even if the holder dies. It returns
// ErrLockNotAcquired when wait elapses and ctx.Err() when ctx does. Olric locks
// are advisory: use them to avoid duplicate work, not to protect invariants.
func (o *DMap) WithLock(ctx context.Context, key Key, ttl, wait time.Duration, fn func(ctx context.Context) error) error {
	if ttl < time.Millisecond {
		return fmt.Errorf("olric.LockWithTimeout: ttl %v is below the 1 ms granularity and would never expire", ttl)
	}

	lock, err := o.acquire(ctx, key, ttl, wait)
	if err != nil {
		return err
	}
	defer func() {
		if err := lock.Unlock(context.WithoutCancel(ctx)); err != nil {
			o.log.Debug("olric.Unlock: failed to release lock", "key", key, "error", err)
		}
	}()

	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(ctx)
}

// acquire always makes at least one attempt, even when wait is zero.
func (o *DMap) acquire(ctx context.Context, key Key, ttl, wait time.Duration) (olric.LockContext, error) {
	deadline := time.Now().Add(wait)
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 && attempt > 0 {
			return nil, ErrLockNotAcquired
		}
		slice := min(lockSlice, max(remaining, 0))

		lock, err := o.dm.LockWithTimeout(ctx, string(key), ttl, slice)
		switch {
		case err == nil:
			return lock, nil
		case isLockNotAcquired(err):
			continue
		default:
			return nil, fmt.Errorf("olric.LockWithTimeout: %w", err)
		}
	}
}
