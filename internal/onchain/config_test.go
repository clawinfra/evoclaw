package onchain

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestChainsStoreAddAndGet(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "chains.json")
	s := NewChainsStore(tmp)

	cfg := ChainConfig{ID: "bsc", Type: "evm", Name: "BSC", RPC: "https://bsc-dataseed.binance.org"}
	s.Add("bsc", cfg)

	got, ok := s.Get("bsc")
	if !ok {
		t.Fatal("expected to get 'bsc' after Add")
	}
	if got.Name != "BSC" {
		t.Errorf("Name = %q, want BSC", got.Name)
	}
}

func TestChainsStoreRemove(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "chains.json")
	s := NewChainsStore(tmp)

	s.Add("eth", ChainConfig{ID: "eth", Type: "evm", Name: "Ethereum", RPC: "https://eth.llamarpc.com"})
	if err := s.Remove("eth"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if _, ok := s.Get("eth"); ok {
		t.Error("expected chain to be removed")
	}
}

func TestChainsStoreRemoveNotFound(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "chains.json")
	s := NewChainsStore(tmp)

	if err := s.Remove("does-not-exist"); err == nil {
		t.Error("expected error when removing non-existent chain")
	}
}

func TestChainsStorePersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "chains.json")
	s := NewChainsStore(path)

	s.Add("polygon", ChainConfig{ID: "polygon", Type: "evm", Name: "Polygon", RPC: "https://polygon-rpc.com"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("chains.json not created: %v", err)
	}

	// Load fresh store
	s2 := NewChainsStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load error: %v", err)
	}

	got, ok := s2.Get("polygon")
	if !ok {
		t.Fatal("chain 'polygon' not found after reload")
	}
	if got.RPC != "https://polygon-rpc.com" {
		t.Errorf("RPC = %q, want https://polygon-rpc.com", got.RPC)
	}
}

func TestChainsStoreLoadMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "chains.json")
	s := NewChainsStore(path)
	if err := s.Load(); err != nil {
		t.Fatalf("Load of missing file should not error, got: %v", err)
	}
	list := s.List()
	if len(list) != 0 {
		t.Errorf("expected empty list for new store, got %d entries", len(list))
	}
}

func TestChainsStoreList(t *testing.T) {
	s := NewChainsStore(filepath.Join(t.TempDir(), "chains.json"))
	s.Add("a", ChainConfig{ID: "a", Type: "evm", Name: "A", RPC: "http://a"})
	s.Add("b", ChainConfig{ID: "b", Type: "evm", Name: "B", RPC: "http://b"})
	s.Add("c", ChainConfig{ID: "c", Type: "substrate", Name: "C", RPC: "wss://c"})

	list := s.List()
	if len(list) != 3 {
		t.Errorf("List() len = %d, want 3", len(list))
	}
}

func TestChainsStoreConcurrentAccess(t *testing.T) {
	s := NewChainsStore(filepath.Join(t.TempDir(), "chains.json"))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := filepath.Join("chain", string(rune('a'+i)))
			s.Add(id, ChainConfig{ID: id, Type: "evm", Name: id, RPC: "http://x"})
			s.List()
		}(i)
	}
	wg.Wait()
}

func TestChainsStoreLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chains.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewChainsStore(path)
	if err := s.Load(); err == nil {
		t.Error("expected error loading invalid JSON")
	}
}

func TestChainsStoreSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "chains.json")
	s := NewChainsStore(path)

	s.Add("base", ChainConfig{ID: "base", Type: "evm", Name: "Base", RPC: "https://mainnet.base.org"})
	s.Add("clawchain", ChainConfig{ID: "clawchain", Type: "substrate", Name: "ClawChain", RPC: "wss://testnet.clawchain.win:9944"})

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s2 := NewChainsStore(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	list := s2.List()
	if len(list) != 2 {
		t.Errorf("expected 2 chains, got %d", len(list))
	}
}

func TestDefaultChainsStoreIsSingleton(t *testing.T) {
	a := DefaultChainsStore()
	b := DefaultChainsStore()
	if a != b {
		t.Error("DefaultChainsStore should return singleton")
	}
}
