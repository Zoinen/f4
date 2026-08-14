package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const connectionFileVersion = 1

var errConnectionCASMiss = errors.New("cloudfox: connection compare-and-swap did not match")

type connectionDocument struct {
	Version     int          `json:"version"`
	Revision    uint64       `json:"revision"`
	Connections []Connection `json:"connections"`
}

var storeLocks sync.Map // absolute metadata path -> *sync.Mutex

func processStoreLock(path string) *sync.Mutex {
	value, _ := storeLocks.LoadOrStore(path, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// ConnectionStore owns only non-secret profile metadata.
type ConnectionStore struct {
	path string
	mu   *sync.Mutex
}

func NewConnectionStore(path string) *ConnectionStore {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return &ConnectionStore{path: path, mu: processStoreLock(path)}
}

func (s *ConnectionStore) Path() string { return s.path }

func (s *ConnectionStore) List(ctx context.Context) ([]Connection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, err := s.readUnlocked()
	if err != nil {
		return nil, err
	}
	out := cloneConnections(doc.Connections)
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if li == lj {
			return out[i].ID < out[j].ID
		}
		return li < lj
	})
	return out, nil
}

func (s *ConnectionStore) Get(ctx context.Context, id string) (Connection, error) {
	items, err := s.List(ctx)
	if err != nil {
		return Connection{}, err
	}
	for _, item := range items {
		if strings.EqualFold(item.ID, id) {
			return item.Clone(), nil
		}
	}
	return Connection{}, ErrConnectionNotFound
}

func (s *ConnectionStore) Create(ctx context.Context, c Connection) (Connection, error) {
	if c.ID == "" {
		id, err := newUUID()
		if err != nil {
			return Connection{}, err
		}
		c.ID = id
	}
	c.ID = strings.ToLower(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now().UTC()
	}
	if err := validateConnection(c); err != nil {
		return Connection{}, err
	}
	err := s.update(ctx, func(doc *connectionDocument) error {
		for _, existing := range doc.Connections {
			if strings.EqualFold(existing.ID, c.ID) {
				return fmt.Errorf("cloudfox: connection id %q already exists", c.ID)
			}
			if strings.EqualFold(existing.Name, c.Name) {
				return fmt.Errorf("%w: %s", ErrDuplicateName, c.Name)
			}
		}
		doc.Connections = append(doc.Connections, c.Clone())
		return nil
	})
	if err != nil {
		return Connection{}, err
	}
	return c.Clone(), nil
}

// Update replaces a profile while preserving its stable ID. The provider type
// cannot change because doing so would reinterpret persisted URIs and secrets.
func (s *ConnectionStore) Update(ctx context.Context, c Connection) (Connection, error) {
	c.ID = strings.ToLower(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	c.UpdatedAt = time.Now().UTC()
	if err := validateConnection(c); err != nil {
		return Connection{}, err
	}
	err := s.update(ctx, func(doc *connectionDocument) error {
		idx := -1
		for i, existing := range doc.Connections {
			if strings.EqualFold(existing.ID, c.ID) {
				idx = i
				if existing.Provider != c.Provider {
					return errors.New("cloudfox: provider type cannot be changed")
				}
				continue
			}
			if strings.EqualFold(existing.Name, c.Name) {
				return fmt.Errorf("%w: %s", ErrDuplicateName, c.Name)
			}
		}
		if idx < 0 {
			return ErrConnectionNotFound
		}
		doc.Connections[idx] = c.Clone()
		return nil
	})
	if err != nil {
		return Connection{}, err
	}
	return c.Clone(), nil
}

// UpdateIfCurrent replaces a profile only if it is still the exact metadata
// revision the caller edited. This prevents a stale dialog from restoring an
// old secret reference after an OAuth refresh or another process changed it.
func (s *ConnectionStore) UpdateIfCurrent(ctx context.Context, c Connection, expectedUpdatedAt time.Time, expectedSecretRef string) (Connection, error) {
	c.ID = strings.ToLower(c.ID)
	c.Name = strings.TrimSpace(c.Name)
	if err := validateConnection(c); err != nil {
		return Connection{}, err
	}

	var saved Connection
	err := s.update(ctx, func(doc *connectionDocument) error {
		idx := -1
		for i, existing := range doc.Connections {
			if strings.EqualFold(existing.ID, c.ID) {
				idx = i
				if existing.Provider != c.Provider {
					return errors.New("cloudfox: provider type cannot be changed")
				}
				continue
			}
			if strings.EqualFold(existing.Name, c.Name) {
				return fmt.Errorf("%w: %s", ErrDuplicateName, c.Name)
			}
		}
		if idx < 0 {
			return ErrConnectionNotFound
		}
		existing := doc.Connections[idx]
		if existing.SecretRef != expectedSecretRef || !existing.UpdatedAt.Equal(expectedUpdatedAt) {
			return errConnectionCASMiss
		}
		c.UpdatedAt = time.Now().UTC()
		saved = c.Clone()
		doc.Connections[idx] = saved.Clone()
		return nil
	})
	if errors.Is(err, errConnectionCASMiss) {
		return Connection{}, ErrConnectionChanged
	}
	if err != nil {
		return Connection{}, err
	}
	return saved, nil
}

// ReplaceSecretRefIfCurrent atomically rotates a credential reference while
// retaining whatever non-secret metadata is current. A false result means a
// newer authorization won the race and must not be overwritten.
func (s *ConnectionStore) ReplaceSecretRefIfCurrent(ctx context.Context, id, expectedRef string, expectedUpdatedAt time.Time, newRef string) (Connection, bool, error) {
	var saved Connection
	err := s.update(ctx, func(doc *connectionDocument) error {
		for i, existing := range doc.Connections {
			if !strings.EqualFold(existing.ID, id) {
				continue
			}
			if existing.SecretRef != expectedRef || !existing.UpdatedAt.Equal(expectedUpdatedAt) {
				return errConnectionCASMiss
			}
			saved = existing.Clone()
			saved.SecretRef = newRef
			saved.UpdatedAt = time.Now().UTC()
			doc.Connections[i] = saved.Clone()
			return nil
		}
		return ErrConnectionNotFound
	})
	if errors.Is(err, errConnectionCASMiss) {
		return Connection{}, false, nil
	}
	if err != nil {
		return Connection{}, false, err
	}
	return saved, true, nil
}

func (s *ConnectionStore) Delete(ctx context.Context, id string) (Connection, error) {
	var deleted Connection
	err := s.update(ctx, func(doc *connectionDocument) error {
		for i, existing := range doc.Connections {
			if strings.EqualFold(existing.ID, id) {
				deleted = existing.Clone()
				doc.Connections = append(doc.Connections[:i], doc.Connections[i+1:]...)
				return nil
			}
		}
		return ErrConnectionNotFound
	})
	return deleted, err
}

func (s *ConnectionStore) update(ctx context.Context, mutate func(*connectionDocument) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	release, err := acquireFileLock(ctx, s.path+".lock")
	if err != nil {
		return err
	}
	defer release()

	doc, err := s.readUnlocked()
	if err != nil {
		return err
	}
	if err := mutate(&doc); err != nil {
		return err
	}
	doc.Version = connectionFileVersion
	doc.Revision++
	return writeJSONAtomic(s.path, doc)
}

func (s *ConnectionStore) readUnlocked() (connectionDocument, error) {
	doc := connectionDocument{Version: connectionFileVersion}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return doc, nil
	}
	if err != nil {
		return doc, fmt.Errorf("cloudfox: read profiles: %w", err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return connectionDocument{}, fmt.Errorf("cloudfox: damaged profile file %s: %w", s.path, err)
	}
	if doc.Version != connectionFileVersion {
		return connectionDocument{}, fmt.Errorf("cloudfox: unsupported profile version %d", doc.Version)
	}
	seenIDs := make(map[string]struct{}, len(doc.Connections))
	seenNames := make(map[string]struct{}, len(doc.Connections))
	for i := range doc.Connections {
		c := &doc.Connections[i]
		c.ID = strings.ToLower(c.ID)
		if err := validateConnection(*c); err != nil {
			return connectionDocument{}, fmt.Errorf("cloudfox: invalid stored profile %d: %w", i, err)
		}
		idKey, nameKey := strings.ToLower(c.ID), strings.ToLower(c.Name)
		if _, ok := seenIDs[idKey]; ok {
			return connectionDocument{}, fmt.Errorf("cloudfox: duplicate stored id %q", c.ID)
		}
		if _, ok := seenNames[nameKey]; ok {
			return connectionDocument{}, fmt.Errorf("cloudfox: duplicate stored name %q", c.Name)
		}
		seenIDs[idKey] = struct{}{}
		seenNames[nameKey] = struct{}{}
	}
	return doc, nil
}

func cloneConnections(items []Connection) []Connection {
	out := make([]Connection, len(items))
	for i := range items {
		out[i] = items[i].Clone()
	}
	return out
}

func acquireFileLock(ctx context.Context, lockPath string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o700); err != nil {
		return nil, fmt.Errorf("cloudfox: create profile directory: %w", err)
	}
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		release, acquired, err := tryAdvisoryFileLock(lockPath)
		if err != nil {
			return nil, fmt.Errorf("cloudfox: lock profile store: %w", err)
		}
		if acquired {
			return release, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("cloudfox: timed out waiting for profile store lock")
		case <-ticker.C:
		}
	}
}

func writeJSONAtomic(filename string, value any) error {
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cloudfox: create config directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil && !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("cloudfox: protect config directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("cloudfox: encode config: %w", err)
	}
	data = append(data, '\n')
	f, err := os.CreateTemp(dir, ".cloudfox-*.tmp")
	if err != nil {
		return fmt.Errorf("cloudfox: create temporary config: %w", err)
	}
	tmp := f.Name()
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("cloudfox: protect temporary config: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("cloudfox: write temporary config: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("cloudfox: sync temporary config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("cloudfox: close temporary config: %w", err)
	}
	if err := os.Rename(tmp, filename); err != nil {
		return fmt.Errorf("cloudfox: replace config: %w", err)
	}
	ok = true
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
