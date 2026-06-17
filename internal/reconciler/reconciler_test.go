// SPDX-License-Identifier: Apache-2.0

package reconciler

import (
	"context"
	"testing"
	"time"

	"oblikovati.org/reporting/internal/storage"
)

// fakeStater answers issue state from a fixed map.
type fakeStater struct{ state map[int]string }

func (f fakeStater) IssueState(_ context.Context, n int) (string, error) {
	return f.state[n], nil
}

func TestSweepDeletesClosedKeepsOpen(t *testing.T) {
	st, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	seed(t, st, "openrep", 1)
	seed(t, st, "closedrep", 2)

	r := New(st, fakeStater{state: map[int]string{1: "open", 2: "closed"}}, time.Minute)
	r.Sweep(context.Background())

	refs, _ := st.Reports()
	if len(refs) != 1 || refs[0].ID != "openrep" {
		t.Fatalf("after sweep refs = %+v, want only openrep", refs)
	}
}

func seed(t *testing.T, st *storage.Store, id string, issue int) {
	t.Helper()
	if err := st.SaveScreenshots(id, []byte("X"), nil); err != nil {
		t.Fatalf("SaveScreenshots: %v", err)
	}
	if err := st.SaveIssueMeta(id, storage.IssueMeta{Number: issue}); err != nil {
		t.Fatalf("SaveIssueMeta: %v", err)
	}
}
