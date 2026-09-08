package ocache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hirundohq/ocache"
	"github.com/hirundohq/ocache/testutil"
)

// twoClients returns two independent DMap handles on the same embedded
// cluster, standing in for two olako-core replicas.
func twoClients(t *testing.T) (a, b *ocache.DMap) {
	t.Helper()
	db, err := testutil.RunServer()
	if err != nil {
		t.Fatalf("RunServer: %v", err)
	}
	t.Cleanup(db.Close)

	open := func() *ocache.DMap {
		cl, err := ocache.New(db.Endpoint())
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		dm, err := cl.NewDMap("locks")
		if err != nil {
			t.Fatalf("NewDMap: %v", err)
		}
		return dm
	}
	return open(), open()
}

func TestWithLockRunsFnAndReturnsItsError(t *testing.T) {
	a, _ := twoClients(t)
	boom := errors.New("boom")

	err := a.WithLock(context.Background(), "token", time.Second, time.Second, func(context.Context) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v; want fn's error", err)
	}

	// The lock was released despite the error: a second call acquires at once.
	ran := false
	if err := a.WithLock(context.Background(), "token", time.Second, 100*time.Millisecond, func(context.Context) error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("second WithLock: err = %v, ran = %v; want nil, true", err, ran)
	}
}

func TestWithLockIsExclusiveAcrossClients(t *testing.T) {
	a, b := twoClients(t)
	ctx := context.Background()

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.WithLock(ctx, "token", 5*time.Second, time.Second, func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	called := false
	err := b.WithLock(ctx, "token", 5*time.Second, 200*time.Millisecond, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, ocache.ErrLockNotAcquired) {
		t.Fatalf("b.WithLock while a holds: err = %v; want ErrLockNotAcquired", err)
	}
	if called {
		t.Error("b's fn ran while a held the lock")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("a.WithLock: %v", err)
	}

	if err := b.WithLock(ctx, "token", 5*time.Second, time.Second, func(context.Context) error { called = true; return nil }); err != nil || !called {
		t.Fatalf("b.WithLock after release: err = %v, called = %v; want nil, true", err, called)
	}
}

func TestWithLockExpiresAfterTTL(t *testing.T) {
	a, b := twoClients(t)
	ctx := context.Background()

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.WithLock(ctx, "token", 300*time.Millisecond, time.Second, func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	start := time.Now()
	err := b.WithLock(ctx, "token", time.Second, 2*time.Second, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("b.WithLock after a's ttl: %v", err)
	}
	if waited := time.Since(start); waited < 200*time.Millisecond {
		t.Errorf("b acquired after %v; want to have waited for a's ttl", waited)
	}

	close(release)
	// a's Unlock finds no lock (it expired); that is logged, not returned.
	if err := <-done; err != nil {
		t.Fatalf("a.WithLock: %v", err)
	}
}
