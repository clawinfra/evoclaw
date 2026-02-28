package skillbank

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotFound is returned when a skill or mistake is not found by ID.
var ErrNotFound = errors.New("skillbank: not found")

// ErrDuplicateID is returned when adding a skill with an already-existing ID.
var ErrDuplicateID = errors.New("skillbank: duplicate ID")

// FileStore is a file-backed JSONL store for skills and common mistakes.
// All operations are thread-safe via a read-write mutex.
// Writes are atomic: data is written to a temp file and then renamed.
type FileStore struct {
	mu       sync.RWMutex
	path     string   // path to skills JSONL file
	mpath    string   // path to mistakes JSONL file
	skills   map[string]Skill
	mistakes map[string]CommonMistake
}

// NewFileStore opens (or creates) a file-backed store at the given path.
// An adjacent "_mistakes" file is used for CommonMistake persistence.
// Existing records are loaded into memory on startup.
func NewFileStore(path string) (*FileStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("skillbank: create store dir: %w", err)
	}

	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	mpath := base + "_mistakes" + ext

	fs := &FileStore{
		path:     path,
		mpath:    mpath,
		skills:   make(map[string]Skill),
		mistakes: make(map[string]CommonMistake),
	}

	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

// load reads skills and mistakes from their respective JSONL files.
func (fs *FileStore) load() error {
	if err := loadJSONL(fs.path, func(data []byte) error {
		var s Skill
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		fs.skills[s.ID] = s
		return nil
	}); err != nil {
		return fmt.Errorf("skillbank: load skills: %w", err)
	}

	if err := loadJSONL(fs.mpath, func(data []byte) error {
		var m CommonMistake
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		fs.mistakes[m.ID] = m
		return nil
	}); err != nil {
		return fmt.Errorf("skillbank: load mistakes: %w", err)
	}

	return nil
}

func loadJSONL(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil // empty store is valid
	}
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	return sc.Err()
}

// flush writes all in-memory skills to the JSONL file atomically.
// Must be called with fs.mu held (write lock).
func (fs *FileStore) flush() error {
	if err := writeJSONL(fs.path, func(enc *json.Encoder) error {
		for _, s := range fs.skills {
			if err := enc.Encode(s); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("skillbank: flush skills: %w", err)
	}
	return nil
}

// flushMistakes writes all in-memory mistakes to the JSONL file atomically.
// Must be called with fs.mu held (write lock).
func (fs *FileStore) flushMistakes() error {
	if err := writeJSONL(fs.mpath, func(enc *json.Encoder) error {
		for _, m := range fs.mistakes {
			if err := enc.Encode(m); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return fmt.Errorf("skillbank: flush mistakes: %w", err)
	}
	return nil
}

// writeJSONL atomically writes JSONL content to path via temp file + rename.
func writeJSONL(path string, fn func(*json.Encoder) error) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".skillbank-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	enc := json.NewEncoder(tmp)
	writeErr := fn(enc)
	closeErr := tmp.Close()

	if writeErr != nil {
		_ = os.Remove(tmpName)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return closeErr
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// Add adds a new skill. Returns ErrDuplicateID if the ID is already present.
func (fs *FileStore) Add(skill Skill) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.skills[skill.ID]; exists {
		return ErrDuplicateID
	}
	fs.skills[skill.ID] = skill
	return fs.flush()
}

// Get returns a skill by ID. Returns ErrNotFound if absent.
func (fs *FileStore) Get(id string) (Skill, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	s, ok := fs.skills[id]
	if !ok {
		return Skill{}, ErrNotFound
	}
	return s, nil
}

// List returns all skills. If category is non-empty, only matching skills are returned.
func (fs *FileStore) List(category string) ([]Skill, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	out := make([]Skill, 0, len(fs.skills))
	for _, s := range fs.skills {
		if category == "" || s.Category == category {
			out = append(out, s)
		}
	}
	return out, nil
}

// Update overwrites an existing skill. Returns ErrNotFound if absent.
func (fs *FileStore) Update(skill Skill) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.skills[skill.ID]; !exists {
		return ErrNotFound
	}
	fs.skills[skill.ID] = skill
	return fs.flush()
}

// Delete removes a skill by ID. Returns ErrNotFound if absent.
func (fs *FileStore) Delete(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.skills[id]; !exists {
		return ErrNotFound
	}
	delete(fs.skills, id)
	return fs.flush()
}

// Count returns the number of stored skills.
func (fs *FileStore) Count() int {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return len(fs.skills)
}

// AddMistake adds a new common mistake.
func (fs *FileStore) AddMistake(m CommonMistake) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.mistakes[m.ID]; exists {
		return ErrDuplicateID
	}
	fs.mistakes[m.ID] = m
	return fs.flushMistakes()
}

// ListMistakes returns common mistakes. If taskType is non-empty, only matching ones are returned.
func (fs *FileStore) ListMistakes(taskType string) ([]CommonMistake, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	out := make([]CommonMistake, 0, len(fs.mistakes))
	for _, m := range fs.mistakes {
		if taskType == "" || m.TaskType == taskType {
			out = append(out, m)
		}
	}
	return out, nil
}

// DeleteMistake removes a mistake by ID.
func (fs *FileStore) DeleteMistake(id string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.mistakes[id]; !exists {
		return ErrNotFound
	}
	delete(fs.mistakes, id)
	return fs.flushMistakes()
}
