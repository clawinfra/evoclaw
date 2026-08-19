# Memory retrieval reproduction

This directory holds the committed dataset and instructions to reproduce EvoClaw's
memory-retrieval comparison — the assets underlying the paper's memory evaluation
(Table II) that were previously missing from the repository.

## Run

```bash
make repro-memory
# or:
go run ./cmd/memory-bench -k 5 -dataset experiments/memory/dataset.json
```

The harness indexes `dataset.json` into the hybrid store using the dependency-free
**local feature-hashing embedder** and reports `recall@k` for three retrieval
modes — keyword-only (FTS5), vector-only (cosine over local embeddings), and
hybrid (merged) — broken out by single-hop and multi-hop queries.

## What this establishes

- **The vector path is functional.** The store previously shipped a no-op embedder
  for every configuration, so vector search silently returned nothing (keyword-only
  in practice). With the local embedder enabled by default, the vector and hybrid
  rows are non-zero and reproducible.
- **Reproducibility.** The dataset, queries, and gold labels are committed; results
  are deterministic and reproduce byte-for-byte offline (no network, no LLM).

## Scope and honesty

- `dataset.json` is **self-generated synthetic data**, as stated in the paper. It is
  small by design (14 documents, 10 queries) so the reproduction is fast and fully
  inspectable.
- This reproduction compares keyword/vector/hybrid retrieval. It does **not**
  reproduce the paper's LLM-navigated **tree-search** numbers, which require an LLM
  and are therefore not part of this offline, deterministic harness.
- A run against a **public multi-hop benchmark** (e.g., HotpotQA / MuSiQue) is the
  planned external-validity extension: convert the benchmark into the same
  `{documents, queries[{gold_docs}]}` schema and pass it via `-dataset`.

## Dataset schema

```json
{
  "documents": [{"id": "d01", "title": "...", "text": "..."}],
  "queries":   [{"id": "q1", "hop": "single|multi", "question": "...", "gold_docs": ["d01"]}]
}
```
