package procfs

import (
	"container/list"
	_ "crypto/sha256"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"

	"emperror.dev/errors"
	"github.com/opencontainers/go-digest"
)

// maxCachedBinaryHashes bounds the number of entries kept in the binary hash
// cache to avoid unbounded memory growth on hosts that run many short-lived
// or distinct binaries.
const maxCachedBinaryHashes = 1024

// binaryCacheKey identifies a binary by its mount namespace plus its
// underlying device/inode rather than by path, since /proc/<PID>/exe may
// resolve to different files across mount namespaces even when the path
// string is the same, and device numbers are only guaranteed unique within
// a given mount namespace's lifetime (they can be reused after a container
// is torn down).
type binaryCacheKey struct {
	mountNamespace string
	dev            uint64
	ino            uint64
}

// binaryHasher computes SHA256 digests of process binaries, caching results
// by the underlying file identity instead of by path. The cache is an LRU
// bounded to maxCachedBinaryHashes entries.
type binaryHasher struct {
	mu    sync.Mutex
	cache map[binaryCacheKey]*list.Element
	order *list.List // front = most recently used
}

type binaryHashCacheEntry struct {
	key  binaryCacheKey
	hash digest.Digest
}

func newBinaryHasher() *binaryHasher {
	return &binaryHasher{
		cache: map[binaryCacheKey]*list.Element{},
		order: list.New(),
	}
}

// Hash returns the SHA256 digest of the binary backing pid's exe. exePath is
// used as a fallback when /proc/<pid>/exe can no longer be opened directly.
func (h *binaryHasher) Hash(pid int32, exePath string) (digest.Digest, error) {
	file, err := os.Open(filepath.Join(procPath(), strconv.Itoa(int(pid)), "exe"))
	if errors.Is(err, os.ErrNotExist) {
		file, err = os.Open(exePath)
	}
	if err != nil {
		return "", err
	}
	defer file.Close()

	key, hasKey := h.cacheKey(pid, file)
	if hasKey {
		if hash, ok := h.lookup(key); ok {
			return hash, nil
		}
	}

	hash, err := digest.SHA256.FromReader(file)
	if err != nil {
		return "", err
	}

	if hasKey {
		h.store(key, hash)
	}

	return hash, nil
}

func (h *binaryHasher) cacheKey(pid int32, file *os.File) (binaryCacheKey, bool) {
	info, err := file.Stat()
	if err != nil {
		return binaryCacheKey{}, false
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return binaryCacheKey{}, false
	}

	mntns, err := os.Readlink(filepath.Join(procPath(), strconv.Itoa(int(pid)), "ns", "mnt"))
	if err != nil {
		return binaryCacheKey{}, false
	}

	return binaryCacheKey{mountNamespace: mntns, dev: uint64(stat.Dev), ino: stat.Ino}, true
}

func (h *binaryHasher) lookup(key binaryCacheKey) (digest.Digest, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	elem, ok := h.cache[key]
	if !ok {
		return "", false
	}

	h.order.MoveToFront(elem)

	return elem.Value.(*binaryHashCacheEntry).hash, true
}

func (h *binaryHasher) store(key binaryCacheKey, hash digest.Digest) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if elem, ok := h.cache[key]; ok {
		elem.Value.(*binaryHashCacheEntry).hash = hash
		h.order.MoveToFront(elem)
		return
	}

	elem := h.order.PushFront(&binaryHashCacheEntry{key: key, hash: hash})
	h.cache[key] = elem

	if h.order.Len() > maxCachedBinaryHashes {
		oldest := h.order.Back()
		if oldest != nil {
			h.order.Remove(oldest)
			delete(h.cache, oldest.Value.(*binaryHashCacheEntry).key)
		}
	}
}
