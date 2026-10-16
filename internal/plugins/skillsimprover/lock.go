package skillsimprover

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"

	"github.com/usewhale/whale/internal/securefs"
)

// targetLocks serializes work on one target inside this process, so two
// proposals for the same file cannot both pass the SHA check and interleave
// their writes (M1-22).
var targetLocks sync.Map

// targetLockKey identifies the real project root and the real target path, so
// two spellings of the same file share one lock.
func (s *Store) targetLockKey(target string) string {
	return resolvePathLenient(s.workspaceRoot) + "\x00" + resolvePathLenient(target)
}

// lockTarget holds the in-process lock for a target and then the cross-process
// lock file, so only one applier at a time touches that target (M1-22, M1-23).
func (s *Store) lockTarget(key string) (func(), error) {
	muAny, _ := targetLocks.LoadOrStore(key, &sync.Mutex{})
	mu := muAny.(*sync.Mutex)
	mu.Lock()

	locksDir := filepath.Join(s.dataDir, "locks")
	if err := securefs.MkdirPrivate(locksDir); err != nil && !os.IsExist(err) {
		mu.Unlock()
		return nil, err
	}
	sum := sha256.Sum256([]byte(key))
	fileUnlock, err := lockFile(filepath.Join(locksDir, hex.EncodeToString(sum[:16])+".lock"))
	if err != nil {
		mu.Unlock()
		return nil, err
	}
	return func() {
		fileUnlock()
		mu.Unlock()
	}, nil
}
