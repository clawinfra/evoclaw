// Command memory-bench reproduces EvoClaw's memory-retrieval comparison on a
// committed dataset. It indexes the documents in experiments/memory/dataset.json
// into the hybrid store using the dependency-free local embedder, then runs each
// query in three retrieval modes — keyword-only (FTS5), vector-only (cosine over
// local embeddings), and hybrid (merged) — and reports recall@k, broken out by
// single-hop and multi-hop queries.
//
// Usage:
//
//	go run ./cmd/memory-bench                 # or: make repro-memory
//	go run ./cmd/memory-bench -k 5 -dataset path/to/dataset.json
//
// This is a deterministic, offline reproduction (no network, no LLM). It is a
// synthetic benchmark, as in the paper; a public multi-hop benchmark run is a
// documented extension (see experiments/memory/README.md). It does not reproduce
// the paper's LLM-navigated tree-search numbers, which require an LLM.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/clawinfra/evoclaw/internal/memory/hybrid"
)

type document struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type query struct {
	ID       string   `json:"id"`
	Hop      string   `json:"hop"`
	Question string   `json:"question"`
	GoldDocs []string `json:"gold_docs"`
}

type dataset struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Documents   []document `json:"documents"`
	Queries     []query    `json:"queries"`
}

type retriever struct {
	name string
	run  func(ctx context.Context, s *hybrid.Store, q string, k int) ([]hybrid.SearchResult, error)
}

func main() {
	datasetPath := flag.String("dataset", "experiments/memory/dataset.json", "path to the retrieval dataset JSON")
	k := flag.Int("k", 5, "retrieval cutoff k for recall@k")
	flag.Parse()

	raw, err := os.ReadFile(*datasetPath)
	if err != nil {
		fatalf("read dataset: %v", err)
	}
	var ds dataset
	if err := json.Unmarshal(raw, &ds); err != nil {
		fatalf("parse dataset: %v", err)
	}

	store, err := hybrid.New(hybrid.Config{
		DBPath:            ":memory:",
		EmbeddingProvider: "local",
		VectorWeight:      0.7,
		KeywordWeight:     0.3,
		ChunkSize:         512,
		ChunkOverlap:      50,
		CacheSize:         256,
	})
	if err != nil {
		fatalf("create store: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	for _, d := range ds.Documents {
		if err := store.Store(ctx, d.ID, d.Title+". "+d.Text, nil); err != nil {
			fatalf("index %s: %v", d.ID, err)
		}
	}

	retrievers := []retriever{
		{"keyword", func(ctx context.Context, s *hybrid.Store, q string, k int) ([]hybrid.SearchResult, error) {
			return s.SearchKeyword(ctx, ftsQuery(q), k)
		}},
		{"vector", func(ctx context.Context, s *hybrid.Store, q string, k int) ([]hybrid.SearchResult, error) {
			return s.SearchVector(ctx, q, k)
		}},
		{"hybrid", func(ctx context.Context, s *hybrid.Store, q string, k int) ([]hybrid.SearchResult, error) {
			return s.Search(ctx, q, k)
		}},
	}

	fmt.Printf("EvoClaw memory-retrieval reproduction — dataset %q\n", ds.Name)
	fmt.Printf("%d documents, %d queries, recall@%d, local feature-hashing embedder\n", len(ds.Documents), len(ds.Queries), *k)
	fmt.Println("=====================================================================")

	type agg struct{ all, single, multi float64 }
	counts := countHops(ds.Queries)

	results := map[string]agg{}
	for _, r := range retrievers {
		var a agg
		for _, q := range ds.Queries {
			hits, err := r.run(ctx, store, q.Question, *k)
			if err != nil {
				fatalf("%s search %s: %v", r.name, q.ID, err)
			}
			rec := recallAtK(q.GoldDocs, hits, *k)
			a.all += rec
			if q.Hop == "multi" {
				a.multi += rec
			} else {
				a.single += rec
			}
		}
		results[r.name] = a
	}

	fmt.Printf("\n%-10s %12s %12s %12s\n", "mode", "recall@k", "single-hop", "multi-hop")
	fmt.Println("---------------------------------------------------------------------")
	for _, r := range retrievers {
		a := results[r.name]
		fmt.Printf("%-10s %11.1f%% %11.1f%% %11.1f%%\n",
			r.name,
			100*a.all/float64(len(ds.Queries)),
			pct(a.single, counts.single),
			pct(a.multi, counts.multi),
		)
	}

	fmt.Println("\nInterpretation: the vector and hybrid rows are non-zero only because the")
	fmt.Println("local embedder is active — the previously shipped no-op embedder made the")
	fmt.Println("vector path return nothing. Numbers are on committed synthetic data and are")
	fmt.Println("reproducible byte-for-byte via `make repro-memory`.")
}

type hopCounts struct{ single, multi int }

func countHops(qs []query) hopCounts {
	var c hopCounts
	for _, q := range qs {
		if q.Hop == "multi" {
			c.multi++
		} else {
			c.single++
		}
	}
	return c
}

func pct(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return 100 * sum / float64(n)
}

// recallAtK returns the fraction of gold documents that appear in the top-k
// retrieved results (deduplicated by document id).
func recallAtK(gold []string, hits []hybrid.SearchResult, k int) float64 {
	if len(gold) == 0 {
		return 0
	}
	topDocs := map[string]bool{}
	seen := 0
	for _, h := range dedupeByDoc(hits) {
		if seen >= k {
			break
		}
		topDocs[h.DocID] = true
		seen++
	}
	found := 0
	for _, g := range gold {
		if topDocs[g] {
			found++
		}
	}
	return float64(found) / float64(len(gold))
}

// dedupeByDoc collapses chunk-level results to their best-scoring document,
// preserving descending score order.
func dedupeByDoc(hits []hybrid.SearchResult) []hybrid.SearchResult {
	best := map[string]hybrid.SearchResult{}
	order := []string{}
	for _, h := range hits {
		if cur, ok := best[h.DocID]; !ok || h.Score > cur.Score {
			if !ok {
				order = append(order, h.DocID)
			}
			best[h.DocID] = h
		}
	}
	out := make([]hybrid.SearchResult, 0, len(order))
	for _, id := range order {
		out = append(out, best[id])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// ftsQuery converts a natural-language question into a safe FTS5 OR-query:
// lowercase alphanumeric tokens, each quoted, joined with OR. This avoids FTS5
// syntax errors on tokens like "ne-7" and gives keyword retrieval its natural
// any-term matching form.
func ftsQuery(question string) string {
	var toks []string
	cur := make([]rune, 0, 16)
	flush := func() {
		if len(cur) > 0 {
			toks = append(toks, `"`+string(cur)+`"`)
			cur = cur[:0]
		}
	}
	for _, r := range question {
		switch {
		case r >= 'A' && r <= 'Z':
			cur = append(cur, r+('a'-'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	if len(toks) == 0 {
		return question
	}
	out := toks[0]
	for _, t := range toks[1:] {
		out += " OR " + t
	}
	return out
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "memory-bench: "+format+"\n", args...)
	os.Exit(1)
}
