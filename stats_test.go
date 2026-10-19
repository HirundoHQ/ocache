package ocache_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hirundohq/ocache"
	"github.com/hirundohq/ocache/testutil"
)

func TestStatsReportsPerDMapUsage(t *testing.T) {
	db, err := testutil.RunServer()
	if err != nil {
		t.Fatalf("RunServer: %v", err)
	}
	defer db.Close()

	cl, err := ocache.New(db.Endpoint())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dm, err := cl.NewDMap("stats")
	if err != nil {
		t.Fatalf("NewDMap: %v", err)
	}
	for _, k := range []string{"a", "b", "c"} {
		dm.PutEx(context.Background(), dm.CreateKey(k), strings.Repeat("v", 100), time.Minute)
	}

	st, err := cl.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}

	// Figures are per partition; the DMap total is their sum.
	var keys, inuse int
	for _, p := range st.Partitions {
		d := p.DMaps["stats"]
		keys += d.Length
		inuse += d.SlabInfo.Inuse
	}
	if keys != 3 {
		t.Errorf("keys in 'stats': want 3, got %d", keys)
	}
	if inuse <= 300 {
		t.Errorf("inuse bytes: want more than the 300 bytes of values, got %d", inuse)
	}
}

func TestStatsFailsWhenOlricIsGone(t *testing.T) {
	db, err := testutil.RunServer()
	if err != nil {
		t.Fatalf("RunServer: %v", err)
	}
	cl, err := ocache.New(db.Endpoint())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := cl.Stats(ctx); err == nil {
		t.Error("Stats after the server is gone: want an error, got nil")
	}
}

func TestScanMatchesKeyPattern(t *testing.T) {
	db, err := testutil.RunServer()
	if err != nil {
		t.Fatalf("RunServer: %v", err)
	}
	defer db.Close()

	cl, err := ocache.New(db.Endpoint())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dm, err := cl.NewDMap("scan")
	if err != nil {
		t.Fatalf("NewDMap: %v", err)
	}
	for _, k := range []string{"offer_1", "offer_2", "token"} {
		dm.PutEx(context.Background(), dm.CreateKey(k), "v", time.Minute)
	}

	it, err := dm.Scan(context.Background(), "^scan_offer_")
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	defer it.Close()
	var got []string
	for it.Next() {
		got = append(got, it.Key())
	}
	if len(got) != 2 {
		t.Errorf("keys matching the prefix: want 2, got %d (%v)", len(got), got)
	}
}
