package coordination

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGuardExclusionAndRecoveryFence(t *testing.T) {
	root := t.TempDir()
	lease, e := Acquire(root, "host", "recovery", "device")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Acquire(root, "host", "normal", "worker"); e == nil {
		t.Fatal("concurrent mutation admitted")
	}
	if Reconcile(root, "host", "operator", lease.State.Epoch) == nil {
		t.Fatal("live lock stolen")
	}
	epoch := lease.State.Epoch
	if e = lease.Close(false); e != nil {
		t.Fatal(e)
	}
	if _, e = Acquire(root, "host", "recovery", "device"); e == nil {
		t.Fatal("uncertain command replayed")
	}
	if Reconcile(root, "host", "operator", epoch+1) == nil {
		t.Fatal("stale acknowledgement accepted")
	}
	if e = Reconcile(root, "host", "operator", epoch); e != nil {
		t.Fatal(e)
	}
	lease, e = Acquire(root, "host", "normal", "worker")
	if e != nil {
		t.Fatal(e)
	}
	if lease.State.Epoch <= epoch {
		t.Fatal("epoch did not advance")
	}
	if e = lease.Close(true); e != nil {
		t.Fatal(e)
	}
	lease, e = Acquire(root, "host", "recovery", "device")
	if e != nil {
		t.Fatal(e)
	}
	if e = lease.Close(true); e != nil {
		t.Fatal(e)
	}
	if _, e = Acquire(root, "host", "normal", "worker"); e == nil {
		t.Fatal("automation resumed without reconciliation")
	}
	events, cursor, e := Events(root, "host", 0)
	if e != nil || len(events) != cursor || cursor < 6 {
		t.Fatal("missing audit", e)
	}
	again, next, e := Events(root, "host", cursor)
	if e != nil || len(again) != 0 || next != cursor {
		t.Fatal("cursor replay")
	}
}
func TestGuardCannotMutateWithoutAudit(t *testing.T) {
	root := t.TempDir()
	if e := os.Mkdir(filepath.Join(root, "host.audit.jsonl"), 0700); e != nil {
		t.Fatal(e)
	}
	if _, e := Acquire(root, "host", "recovery", "device"); e == nil {
		t.Fatal("audit write failure ignored")
	}
	s, e := Read(root, "host")
	if e != nil || s.Active {
		t.Fatal("authority issued despite failed audit")
	}
}
