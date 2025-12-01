package kubernetes_test

import (
	_ "embed"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gezacorp/metadatax/collectors/kubernetes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedFile_FirstLoad(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/file.pem"
	content := []byte("hello")

	require.NoError(t, os.WriteFile(path, content, 0o600))

	f, err := kubernetes.NewCachedFile(path, time.Minute)
	require.NoError(t, err)
	require.NotNil(t, f)

	b, changed, err := f.Content()
	require.NoError(t, err)

	assert.Equal(t, content, b)
	assert.True(t, changed, "first load must return changed=true")
}

func TestCachedFile_CacheHitWithinLifetime(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/file.pem"
	content := []byte("hello")

	require.NoError(t, os.WriteFile(path, content, 0o600))

	f, err := kubernetes.NewCachedFile(path, time.Hour)
	require.NoError(t, err)

	// prime cache
	_, _, _ = f.Content()

	// second access
	b, changed, err := f.Content()
	require.NoError(t, err)

	assert.Equal(t, content, b)
	assert.False(t, changed, "should not indicate changed within cache lifetime")
}

func TestCachedFile_ReloadAfterLifetime(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/file.pem"

	v1 := []byte("hello")
	v2 := []byte("world")

	require.NoError(t, os.WriteFile(path, v1, 0o600))

	f, err := kubernetes.NewCachedFile(path, 10*time.Millisecond)
	require.NoError(t, err)

	_, _, _ = f.Content() // prime cache

	time.Sleep(20 * time.Millisecond)
	require.NoError(t, os.WriteFile(path, v2, 0o600))

	b, changed, err := f.Content()
	require.NoError(t, err)

	assert.True(t, changed, "expected changed=true after file change + expiration")
	assert.Equal(t, v2, b)
}

func TestCachedFile_ErrorOnMissingFile(t *testing.T) {
	f, err := kubernetes.NewCachedFile("/definitely/not/existing.pem", time.Second)

	require.Error(t, err)
	assert.Nil(t, f)
}

func TestCachedFile_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/file.pem"
	content := []byte("data")

	require.NoError(t, os.WriteFile(path, content, 0o600))

	f, err := kubernetes.NewCachedFile(path, time.Minute)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(20)

	for range 20 {
		go func() {
			defer wg.Done()
			b, _, err := f.Content()
			require.NoError(t, err)
			assert.Equal(t, content, b)
		}()
	}

	wg.Wait()
}

type mockCachedFile struct {
	content []byte
	changed bool
	err     error
}

func (m *mockCachedFile) Content() ([]byte, bool, error) {
	return m.content, m.changed, m.err
}

// -----------------------------------------------------------------------------
// Test Data
// -----------------------------------------------------------------------------
//
//go:embed testdata/test.crt
var testCertPEM []byte

//go:embed testdata/test.key
var testKeyPEM []byte

func TestCachedCertificate_FirstLoad(t *testing.T) {
	certFile := &mockCachedFile{content: testCertPEM, changed: true}
	keyFile := &mockCachedFile{content: testKeyPEM, changed: true}

	c := kubernetes.NewCachedCertificate(certFile, keyFile)

	cert, err := c.Certificate()
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Len(t, cert.Certificate, 1)
}

func TestCachedCertificate_CacheHit(t *testing.T) {
	certFile := &mockCachedFile{content: testCertPEM, changed: true}
	keyFile := &mockCachedFile{content: testKeyPEM, changed: true}

	c := kubernetes.NewCachedCertificate(certFile, keyFile)

	first, err := c.Certificate()
	require.NoError(t, err)

	// Now simulate "no changes"
	certFile.changed = false
	keyFile.changed = false

	second, err := c.Certificate()
	require.NoError(t, err)

	assert.Same(t, first, second, "certificate pointer must be identical when unchanged")
}

func TestCachedCertificate_ReloadOnCertChange(t *testing.T) {
	certFile := &mockCachedFile{content: testCertPEM, changed: true}
	keyFile := &mockCachedFile{content: testKeyPEM, changed: true}

	c := kubernetes.NewCachedCertificate(certFile, keyFile)

	first, err := c.Certificate()
	require.NoError(t, err)

	// Mutate cert → changed
	certFile.changed = true
	certFile.content = append([]byte{}, testCertPEM...) // pretend new content

	second, err := c.Certificate()
	require.NoError(t, err)

	assert.NotSame(t, first, second, "cache must reload certificate on cert change")
}

func TestCachedCertificate_ReloadOnKeyChange(t *testing.T) {
	certFile := &mockCachedFile{content: testCertPEM, changed: true}
	keyFile := &mockCachedFile{content: testKeyPEM, changed: true}

	c := kubernetes.NewCachedCertificate(certFile, keyFile)

	first, err := c.Certificate()
	require.NoError(t, err)

	// Mutate key → changed
	keyFile.changed = true
	keyFile.content = append([]byte{}, testKeyPEM...)

	second, err := c.Certificate()
	require.NoError(t, err)

	assert.NotSame(t, first, second, "cache must reload on key change")
}

func TestCachedCertificate_ErrorOnCertRead(t *testing.T) {
	certFile := &mockCachedFile{err: errors.New("read error")}
	keyFile := &mockCachedFile{}

	c := kubernetes.NewCachedCertificate(certFile, keyFile)

	_, err := c.Certificate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not get certificate")
}

func TestCachedCertificate_ErrorOnKeyRead(t *testing.T) {
	certFile := &mockCachedFile{}
	keyFile := &mockCachedFile{err: errors.New("key error")}

	c := kubernetes.NewCachedCertificate(certFile, keyFile)

	_, err := c.Certificate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not get private key")
}

func TestCachedCertificate_ErrorOnX509Parse(t *testing.T) {
	certFile := &mockCachedFile{content: []byte("invalid"), changed: true}
	keyFile := &mockCachedFile{content: []byte("invalid"), changed: true}

	c := kubernetes.NewCachedCertificate(certFile, keyFile)

	_, err := c.Certificate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not parse x509 key pair")
}
