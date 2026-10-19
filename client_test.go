package ocache_test

import (
	"context"
	"testing"
	"time"

	"github.com/hirundohq/ocache"
	"github.com/hirundohq/ocache/testutil"
)

func TestClient(t *testing.T) {
	db, err := testutil.RunServer()
	if err != nil {
		t.Error(err)
	}
	defer db.Close()

	t.Log("address:", db.Endpoint())

	cl, err := ocache.New(db.Endpoint())
	if err != nil {
		t.Errorf("failed to create client: %v", err)
	}

	dm, err := cl.NewDMap("test")
	if err != nil {
		t.Errorf("failed to create DMap: %v", err)
	}

	dm.PutEx(context.Background(), "key", "value", 1*time.Second)

	value := dm.Get(context.Background(), "key")

	if value == nil {
		t.Errorf("value is nil")
	}

	val, err := value.String()
	if err != nil {
		t.Errorf("failed to convert value to string: %v", err)
	}

	if val != "value" {
		t.Errorf("value is not equal to 'value'")
	}
}

func TestLookupDistinguishesMissFromHit(t *testing.T) {
	db, err := testutil.RunServer()
	if err != nil {
		t.Fatalf("RunServer: %v", err)
	}
	defer db.Close()

	cl, err := ocache.New(db.Endpoint())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dm, err := cl.NewDMap("lookup")
	if err != nil {
		t.Fatalf("NewDMap: %v", err)
	}
	ctx := context.Background()

	if val, found, err := dm.Lookup(ctx, "token"); err != nil || found || val != nil {
		t.Fatalf("Lookup before put = (%v, %v, %v); want (nil, false, nil)", val, found, err)
	}

	dm.PutEx(ctx, "token", "abc", time.Minute)

	val, found, err := dm.Lookup(ctx, "token")
	if err != nil || !found || val == nil {
		t.Fatalf("Lookup after put = (%v, %v, %v); want a hit", val, found, err)
	}
	if got, err := val.String(); err != nil || got != "abc" {
		t.Errorf("value = %q, %v; want abc", got, err)
	}
}
