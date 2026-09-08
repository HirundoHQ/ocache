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

// isLockNotAcquired mirrors isKeyNotFound: a client-only process gets the
// wire error back as a plain string, not the sentinel.
func isLockNotAcquired(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, olric.ErrLockNotAcquired) || err.Error() == olric.ErrLockNotAcquired.Error()
}

// lockSlice bounds one LockWithTimeout round trip. The server polls for the
// whole deadline while the client waits for a single reply under its read
// timeout (3 s by default), and a reply that outlives it is retried, leaving
// server-side pollers that can take the lock into a dead connection. Waiting
// in slices keeps every reply inside the read timeout.
const lockSlice = time.Second

// WithLock runs fn while holding a cluster-wide lock on key. The lock expires
// after ttl even if the holder dies, so ttl must exceed fn's worst case and
// must be at least one millisecond (Olric's granularity; below that the lock
// would never expire). A caller that cannot acquire the lock within wait gets
// ErrLockNotAcquired and fn does not run; a caller whose ctx ends while
// waiting gets ctx.Err(). The wait is served in slices of lockSlice so that
// each reply arrives within the client's read timeout. Unlock failures of any
// kind (an expired lock, an unreachable cluster) are only logged: the lock
// then lapses on its own at ttl. A panic in fn still releases the lock. Olric
// locks are advisory (a key with a TTL, polled every 10 ms): use them to avoid
// duplicate work, not to protect invariants, and never derive key from a
// secret — it is logged at Debug.
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

// acquire waits up to wait for the lock, one lockSlice at a time, so that no
// LockWithTimeout reply outlives the client's read timeout. At least one
// attempt is made even when wait is zero.
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
