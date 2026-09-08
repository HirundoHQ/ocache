package ocache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hirundohq/logging"
	"github.com/hirundohq/logging/fake"
	"github.com/olric-data/olric"
)

// lockingFakeDMap fails LockWithTimeout with lockErr; it never hands out a
// LockContext, so fn must not run.
type lockingFakeDMap struct {
	olric.DMap
	lockErr error
}

func (f *lockingFakeDMap) Name() string { return "test" }

func (f *lockingFakeDMap) LockWithTimeout(_ context.Context, _ string, _, _ time.Duration) (olric.LockContext, error) {
	return nil, f.lockErr
}

func TestWithLockMapsPlainStringLockNotAcquired(t *testing.T) {
	dm := &DMap{dm: &lockingFakeDMap{lockErr: errors.New("lock not acquired")}, log: fake.New(logging.LevelError)}

	called := false
	err := dm.WithLock(context.Background(), "k", time.Second, time.Millisecond, func(context.Context) error {
		called = true
		return nil
	})

	if !errors.Is(err, ErrLockNotAcquired) {
		t.Fatalf("err = %v; want ErrLockNotAcquired", err)
	}
	if called {
		t.Error("fn ran without the lock")
	}
}

func TestWithLockMapsSentinelLockNotAcquired(t *testing.T) {
	dm := &DMap{dm: &lockingFakeDMap{lockErr: olric.ErrLockNotAcquired}, log: fake.New(logging.LevelError)}

	err := dm.WithLock(context.Background(), "k", time.Second, time.Millisecond, func(context.Context) error { return nil })
	if !errors.Is(err, ErrLockNotAcquired) {
		t.Fatalf("err = %v; want ErrLockNotAcquired", err)
	}
}

func TestWithLockReportsClusterFailure(t *testing.T) {
	down := errors.New("dial tcp: connection refused")
	dm := &DMap{dm: &lockingFakeDMap{lockErr: down}, log: fake.New(logging.LevelError)}

	err := dm.WithLock(context.Background(), "k", time.Second, time.Millisecond, func(context.Context) error { return nil })
	if !errors.Is(err, down) {
		t.Fatalf("err = %v; want wrapped %v", err, down)
	}
	if errors.Is(err, ErrLockNotAcquired) {
		t.Error("a cluster failure must not read as a contended lock")
	}
}
