package demo

import (
	"testing"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/store"
)

// A connector created after the seed must not replace a seeded one.
func TestSeedReservesConnectorIDs(t *testing.T) {
	st := store.New()
	Seed(st)
	st.Write(func(d *store.Data) {
		before := len(d.Connectors)
		id := d.NextID("CON")
		if d.Connectors[id] != nil {
			t.Fatalf("NextID returned seeded id %s", id)
		}
		if before == 0 {
			t.Fatal("seed created no connectors")
		}
	})
}
