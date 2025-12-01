package kubernetes

import (
	"bytes"
	"crypto/tls"
	"os"
	"regexp"
	"sync"
	"time"

	"emperror.dev/errors"
	"github.com/prometheus/procfs"
)

type Cgroup = procfs.Cgroup

func GetProc(pid int) (procfs.Proc, error) {
	hostProc := os.Getenv("HOST_PROC")
	if hostProc == "" {
		return procfs.NewProc(pid)
	}

	fs, fsErr := procfs.NewFS(hostProc)
	if fsErr != nil {
		return procfs.Proc{}, errors.WrapIf(fsErr, "could not create a new procfs")
	}

	return fs.Proc(pid)
}

func GetCgroupsForPID(pid int) ([]Cgroup, error) {
	proc, err := GetProc(pid)
	if err != nil {
		return nil, errors.WrapIf(err, "could not get process info")
	}

	cgroups, err := proc.Cgroups()
	if err != nil {
		return nil, errors.WrapIf(err, "could not get cgroups")
	}

	return cgroups, nil
}

func GetContainerIDFromCgroups(cgroups []Cgroup) string {
	dockerCgroupRegex := regexp.MustCompile(`\b((?i)[a-z0-9]{64})`)

	for _, cgroup := range cgroups {
		if match := dockerCgroupRegex.FindStringSubmatch(cgroup.Path); len(match) > 0 {
			return match[1]
		}
	}

	return ""
}

func NodeName() (string, error) {
	return os.Hostname()
}

type CachedCertificate interface {
	Certificate() (*tls.Certificate, error)
}

type cachedCertificate struct {
	cert     *tls.Certificate
	certFile CachedFile
	keyFile  CachedFile
}

func NewCachedCertificate(certFile CachedFile, keyFile CachedFile) CachedCertificate {
	return &cachedCertificate{
		certFile: certFile,
		keyFile:  keyFile,
	}
}

func (c *cachedCertificate) Certificate() (*tls.Certificate, error) {
	certContent, certChanged, err := c.certFile.Content()
	if err != nil {
		return nil, errors.WrapIf(err, "could not get certificate")
	}

	keyContent, keyChanged, err := c.keyFile.Content()
	if err != nil {
		return nil, errors.WrapIf(err, "could not get private key")
	}

	if c.cert == nil || certChanged || keyChanged {
		cert, err := tls.X509KeyPair(certContent, keyContent)
		if err != nil {
			return nil, errors.WrapIf(err, "could not parse x509 key pair")
		}

		c.cert = &cert
	}

	return c.cert, nil
}

type CachedFile interface {
	Content() ([]byte, bool, error)
}

type cachedFileContent struct {
	path     string
	content  []byte
	setAt    time.Time
	lifetime time.Duration

	mu sync.RWMutex
}

func NewCachedFile(path string, lifetime time.Duration) (*cachedFileContent, error) {
	f := &cachedFileContent{
		path:     path,
		setAt:    time.Now(),
		lifetime: lifetime,
	}

	if _, _, err := f.Content(); err != nil {
		return nil, err
	}
	f.content = nil

	return f, nil
}

func (f *cachedFileContent) Content() ([]byte, bool, error) {
	f.mu.RLock()
	if f.content != nil && time.Since(f.setAt) < f.lifetime {
		f.mu.RUnlock()

		return f.content, false, nil
	}
	content := f.content
	f.mu.RUnlock()

	c, err := os.ReadFile(f.path)
	if err != nil {
		return nil, false, errors.WithStackIf(err)
	}

	changed := !bytes.Equal(c, content)
	f.mu.Lock()
	f.content = c
	f.mu.Unlock()

	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.content, changed, nil
}
