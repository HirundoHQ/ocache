package ocache

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hirundohq/logging"
	"github.com/hirundohq/logging/fake"
	"github.com/olric-data/olric"
)

type fakeOlricDMap struct {
	olric.DMap
	getErr    error
	deleteErr error
}

func (f *fakeOlricDMap) Name() string { return "test" }

func (f *fakeOlricDMap) Get(_ context.Context, _ string) (*olric.GetResponse, error) {
	return nil, f.getErr
}

func (f *fakeOlricDMap) Delete(_ context.Context, _ ...string) (int, error) {
	return 0, f.deleteErr
}

func assertNoErrorLogs(t *testing.T, log *fake.Logger) {
	t.Helper()

	entries, err := log.GetTestLogs()
	if err != nil {
		t.Fatalf("failed to read fake logs: %v", err)
	}
	for _, e := range entries {
		if e.GetLevel() == logging.LevelError {
			t.Errorf("unexpected error log on key-not-found: %s", e.GetMessage())
		}
	}
}

func TestCreateKeyEmptyOptionsMatchesNoOptions(t *testing.T) {
	dm := &DMap{dm: &fakeOlricDMap{}}

	got := dm.CreateKey("offers", []KeyOption{}...)
	want := dm.CreateKey("offers")

	if got != want {
		t.Errorf("CreateKey with empty options slice = %q, want %q", got, want)
	}
}

func TestGetDoesNotLogErrorOnWrappedKeyNotFound(t *testing.T) {
	log := fake.New(logging.LevelError)
	dm := &DMap{
		dm:  &fakeOlricDMap{getErr: fmt.Errorf("cluster client: %w", olric.ErrKeyNotFound)},
		log: log,
	}

	if val := dm.Get(context.Background(), "missing"); val != nil {
		t.Errorf("expected nil response for missing key, got %v", val)
	}

	assertNoErrorLogs(t, log)
}

// A ClusterClient in a process without an in-process olric server gets wire
// errors back as plain strings ("key not found"), not the sentinel — see
// olric's processProtocolError and the prefix registry populated only by
// olric.New. These tests pin that production shape.
func TestGetDoesNotLogErrorOnPlainStringKeyNotFound(t *testing.T) {
	log := fake.New(logging.LevelError)
	dm := &DMap{
		dm:  &fakeOlricDMap{getErr: errors.New("key not found")},
		log: log,
	}

	if val := dm.Get(context.Background(), "missing"); val != nil {
		t.Errorf("expected nil response for missing key, got %v", val)
	}

	assertNoErrorLogs(t, log)
}

func TestDeleteDoesNotLogErrorOnPlainStringKeyNotFound(t *testing.T) {
	log := fake.New(logging.LevelError)
	dm := &DMap{
		dm:  &fakeOlricDMap{deleteErr: errors.New("key not found")},
		log: log,
	}

	dm.Delete(context.Background(), "missing")

	assertNoErrorLogs(t, log)
}

func TestDeleteDoesNotLogErrorOnWrappedKeyNotFound(t *testing.T) {
	log := fake.New(logging.LevelError)
	dm := &DMap{
		dm:  &fakeOlricDMap{deleteErr: fmt.Errorf("cluster client: %w", olric.ErrKeyNotFound)},
		log: log,
	}

	dm.Delete(context.Background(), "missing")

	assertNoErrorLogs(t, log)
}

func TestLookupTreatsSentinelKeyNotFoundAsMiss(t *testing.T) {
	log := fake.New(logging.LevelError)
	dm := &DMap{
		dm:  &fakeOlricDMap{getErr: fmt.Errorf("cluster client: %w", olric.ErrKeyNotFound)},
		log: log,
	}

	val, found, err := dm.Lookup(context.Background(), "missing")
	if err != nil {
		t.Fatalf("Lookup returned error for a miss: %v", err)
	}
	if found || val != nil {
		t.Errorf("Lookup = (%v, %v); want (nil, false) for a miss", val, found)
	}
	assertNoErrorLogs(t, log)
}

func TestLookupTreatsPlainStringKeyNotFoundAsMiss(t *testing.T) {
	dm := &DMap{dm: &fakeOlricDMap{getErr: errors.New("key not found")}, log: fake.New(logging.LevelError)}

	_, found, err := dm.Lookup(context.Background(), "missing")
	if err != nil || found {
		t.Errorf("Lookup = (found=%v, err=%v); want (false, nil) for the wire-string miss", found, err)
	}
}

func TestLookupReportsClusterFailure(t *testing.T) {
	down := errors.New("dial tcp 127.0.0.1:3320: connect: connection refused")
	dm := &DMap{dm: &fakeOlricDMap{getErr: down}, log: fake.New(logging.LevelError)}

	val, found, err := dm.Lookup(context.Background(), "token")
	if !errors.Is(err, down) {
		t.Fatalf("Lookup err = %v; want wrapped %v", err, down)
	}
	if found || val != nil {
		t.Errorf("Lookup = (%v, %v) on failure; want (nil, false)", val, found)
	}
}
