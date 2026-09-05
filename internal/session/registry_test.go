package session

import "testing"

func TestRegistry_CreateThenGet(t *testing.T) {
	r := NewRegistry()
	id, ch := r.Create()
	if id == "" {
		t.Fatal("expected non-empty id")
	}
	got, ok := r.Get(id)
	if !ok {
		t.Fatal("expected ok=true for known id")
	}
	if got != ch {
		t.Fatal("expected Get to return the channel from Create")
	}
}

func TestRegistry_GetUnknown(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Get("does-not-exist"); ok {
		t.Fatal("expected ok=false for unknown id")
	}
}

func TestRegistry_Delete(t *testing.T) {
	r := NewRegistry()
	id, _ := r.Create()
	r.Delete(id)
	if _, ok := r.Get(id); ok {
		t.Fatal("expected ok=false after Delete")
	}
}
