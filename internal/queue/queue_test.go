// SPDX-License-Identifier: Apache-2.0

package queue

import "testing"

func TestEnqueueDeliversFIFO(t *testing.T) {
	q := New(2)
	if !q.Enqueue(Job{ID: "a"}) || !q.Enqueue(Job{ID: "b"}) {
		t.Fatal("enqueue into capacity should succeed")
	}
	if got := <-q.Jobs(); got.ID != "a" {
		t.Errorf("first job = %q, want a", got.ID)
	}
	if got := <-q.Jobs(); got.ID != "b" {
		t.Errorf("second job = %q, want b", got.ID)
	}
}

func TestEnqueueReportsFullWithoutBlocking(t *testing.T) {
	q := New(1)
	if !q.Enqueue(Job{ID: "a"}) {
		t.Fatal("first enqueue should succeed")
	}
	if q.Enqueue(Job{ID: "b"}) {
		t.Error("enqueue into a full queue should return false, not block")
	}
}
