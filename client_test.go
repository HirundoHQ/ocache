package ocache_test

import (
	"context"
	"testing"
	"time"

	client "github.com/hirundohq/ocache"
	"github.com/hirundohq/ocache/testutil"
)

func TestClient(t *testing.T) {
	db, err := testutil.RunServer()
	if err != nil {
		t.Error(err)
	}
	defer db.Close()

	t.Log("address:", db.Endpoint())

	cl, err := client.New(db.Endpoint())
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
