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

// WithLock runs fn while holding a cluster-wide lock on key. The lock expires
// after ttl even if the holder dies, so ttl must exceed fn's worst case; a
// caller that cannot acquire it within wait gets ErrLockNotAcquired and fn
// does not run. Olric locks are advisory (a key with a TTL, polled every
// 10 ms): use them to avoid duplicate work, not to protect invariants.
func (o *DMap) WithLock(ctx context.Context, key Key, ttl, wait time.Duration, fn func(ctx context.Context) error) error {
	lock, err := o.dm.LockWithTimeout(ctx, string(key), ttl, wait)
	if err != nil {
		if isLockNotAcquired(err) {
			return ErrLockNotAcquired
		}
		return fmt.Errorf("olric.LockWithTimeout: %w", err)
	}
	defer func() {
		// A lock that already expired reports "no such lock"; nothing to do.
		if err := lock.Unlock(context.WithoutCancel(ctx)); err != nil {
			o.log.Debug("olric.Unlock: failed to release lock", "key", key, "error", err)
		}
	}()

	return fn(ctx)
}
