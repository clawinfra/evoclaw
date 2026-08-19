package main

import (
	"testing"

	"github.com/clawinfra/evoclaw/internal/memory/hybrid"
)

func TestRecallAtK(t *testing.T) {
	hits := []hybrid.SearchResult{
		{DocID: "d02", Score: 0.9},
		{DocID: "d01", Score: 0.8},
		{DocID: "d09", Score: 0.7},
	}
	cases := []struct {
		name string
		gold []string
		k    int
		want float64
	}{
		{"both gold in top-2", []string{"d01", "d02"}, 2, 1.0},
		{"one of two gold in top-1", []string{"d02", "d09"}, 1, 0.5},
		{"none present", []string{"d13"}, 3, 0.0},
		{"empty gold", nil, 3, 0.0},
	}
	for _, c := range cases {
		if got := recallAtK(c.gold, hits, c.k); got != c.want {
			t.Errorf("%s: recallAtK=%.2f want %.2f", c.name, got, c.want)
		}
	}
}

func TestDedupeByDoc(t *testing.T) {
	hits := []hybrid.SearchResult{
		{DocID: "d1", ChunkID: "d1-0", Score: 0.3},
		{DocID: "d1", ChunkID: "d1-1", Score: 0.9},
		{DocID: "d2", ChunkID: "d2-0", Score: 0.5},
	}
	out := dedupeByDoc(hits)
	if len(out) != 2 {
		t.Fatalf("expected 2 docs after dedupe, got %d", len(out))
	}
	if out[0].DocID != "d1" || out[0].Score != 0.9 {
		t.Errorf("expected d1@0.9 first, got %s@%.2f", out[0].DocID, out[0].Score)
	}
}

func TestFTSQuery(t *testing.T) {
	got := ftsQuery("How often does ne-7 report?")
	want := `"how" OR "often" OR "does" OR "ne" OR "7" OR "report"`
	if got != want {
		t.Errorf("ftsQuery mismatch:\n got %s\nwant %s", got, want)
	}
	if ftsQuery("!!! ???") != "!!! ???" {
		t.Error("token-free input should be returned unchanged")
	}
}
