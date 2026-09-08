package ocache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hirundohq/ocache"
	"github.com/hirundohq/ocache/testutil"
)

// newServer starts an embedded Olric cluster of one node.
func newServer(t *testing.T) *testutil.OlricServer {
	t.Helper()
	db, err := testutil.RunServer()
	if err != nil {
		t.Fatalf("RunServer: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

// Two handles on the same server stand in for two replicas.
func openDMap(t *testing.T, db *testutil.OlricServer) *ocache.DMap {
	t.Helper()
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

func TestWithLockRunsFnAndReturnsItsError(t *testing.T) {
	a := openDMap(t, newServer(t))
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

func TestWithLockRejectsSubMillisecondTTL(t *testing.T) {
	a := openDMap(t, newServer(t))

	called := false
	err := a.WithLock(context.Background(), "token", 500*time.Microsecond, time.Second, func(context.Context) error { called = true; return nil })
	if err == nil {
		t.Fatal("WithLock accepted a ttl that Olric would never expire")
	}
	if called {
		t.Error("fn ran despite the rejected ttl")
	}
}

func TestWithLockIsExclusiveAcrossClients(t *testing.T) {
	db := newServer(t)
	a, b := openDMap(t, db), openDMap(t, db)
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

// A wait longer than the Olric client's read timeout used to fail with an
// opaque i/o timeout and leave the lock orphaned by a retried LOCK command.
func TestWithLockWaitsBeyondClientReadTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("contended wait of several seconds")
	}
	db := newServer(t)
	a, b := openDMap(t, db), openDMap(t, db)
	ctx := context.Background()

	held := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.WithLock(ctx, "token", 30*time.Second, time.Second, func(context.Context) error {
			close(held)
			time.Sleep(4 * time.Second) // longer than the client's 3 s read timeout
			return nil
		})
	}()
	<-held

	start := time.Now()
	if err := b.WithLock(ctx, "token", 30*time.Second, 10*time.Second, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("b.WithLock after a contended wait beyond the read timeout: %v", err)
	}
	if waited := time.Since(start); waited < 3500*time.Millisecond {
		t.Errorf("b acquired after %v; want to have waited for a's 4 s", waited)
	}
	if err := <-done; err != nil {
		t.Fatalf("a.WithLock: %v", err)
	}

	// No retried LOCK command left a poller holding the key: a third acquisition is immediate.
	start = time.Now()
	if err := a.WithLock(ctx, "token", 30*time.Second, time.Second, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("third WithLock: %v (the lock is orphaned)", err)
	}
	if waited := time.Since(start); waited > time.Second {
		t.Errorf("third acquisition took %v; want immediate", waited)
	}
}

func TestWithLockExpiresAfterTTL(t *testing.T) {
	db := newServer(t)
	a, b := openDMap(t, db), openDMap(t, db)
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
	// a's Unlock fails silently: the lock already expired.
	if err := <-done; err != nil {
		t.Fatalf("a.WithLock: %v", err)
	}
}

// ctx is checked at each slice boundary, so this can take up to about 1.3 s.
func TestWithLockReturnsContextErrorWhileWaiting(t *testing.T) {
	db := newServer(t)
	a, b := openDMap(t, db), openDMap(t, db)

	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.WithLock(context.Background(), "token", 5*time.Second, time.Second, func(context.Context) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	called := false
	err := b.WithLock(ctx, "token", 5*time.Second, 10*time.Second, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("b.WithLock with an expiring context: err = %v; want DeadlineExceeded", err)
	}
	if called {
		t.Error("fn ran after the context expired")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("a.WithLock: %v", err)
	}
}

func TestWithLockRejectsCancelledContextUpFront(t *testing.T) {
	a := openDMap(t, newServer(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	called := false
	err := a.WithLock(ctx, "token", time.Second, time.Second, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("err = %v, called = %v; want context.Canceled and fn not run", err, called)
	}
}
