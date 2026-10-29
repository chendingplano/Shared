package iconlib

import "testing"

func TestFuseRanksSharedHitsFirst(t *testing.T) {
	a := []Icon{{Name: "x"}, {Name: "y"}, {Name: "z"}}
	b := []Icon{{Name: "y"}, {Name: "w"}}
	got := fuse([][]Icon{a, b}, 3)
	if got[0].Name != "y" {
		t.Fatalf("expected y first (in both lists), got %v", got)
	}
	if len(got) != 3 {
		t.Fatalf("limit not applied: %d", len(got))
	}
}

func TestFuseSingleListKeepsOrder(t *testing.T) {
	got := fuse([][]Icon{{{Name: "a"}, {Name: "b"}}}, 10)
	if got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("order changed: %v", got)
	}
}
