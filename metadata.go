package metadatax

import (
	"strings"
	"sync"
)

const (
	defaultLevelSeparator = ":"
)

type MetadataContainer interface {
	MetadataLabels
}

type MetadataLabels interface {
	GetLabels() Labels
	GetLabelsSlice() []SlicedLabel
	AddLabel(name string, value ...string)
	AddLabels(Labels)
	Level(name string, opts ...MetadataOption) MetadataContainer
}

type Labels map[string][]string

type SlicedLabel struct {
	Name  string
	Value string
}

type MetadataOption func(*metadata)

type metadataOpts struct {
	prefix          string
	levelSeparator  string
	storeOnSubLevel bool
	uniqueValues    bool
	uniqueKeys      bool
}

type metadata struct {
	MetadataContainer
	Labels Labels

	opts metadataOpts

	mu *sync.RWMutex
}

func withMetadataOpts(opts metadataOpts) MetadataOption {
	return func(m *metadata) {
		if m == nil {
			return
		}

		m.opts = opts
	}
}

func WithMetadata(md MetadataContainer) MetadataOption {
	return func(m *metadata) {
		if m == nil {
			return
		}

		m.MetadataContainer = md
	}
}

func WithPrefix(prefix string) MetadataOption {
	return func(m *metadata) {
		m.opts.prefix = prefix
	}
}

func WithLevelSeparator(separator string) MetadataOption {
	return func(m *metadata) {
		m.opts.levelSeparator = separator
	}
}

func WithStoreOnSubLevel(store bool) MetadataOption {
	return func(m *metadata) {
		m.opts.storeOnSubLevel = store
	}
}

func WithConcurrencySupport() MetadataOption {
	return func(m *metadata) {
		m.mu = &sync.RWMutex{}
	}
}

func WithUniqueValues(enabled bool) MetadataOption {
	return func(m *metadata) {
		m.opts.uniqueValues = enabled
	}
}

func WithUniqueKeys(enabled bool) MetadataOption {
	return func(m *metadata) {
		m.opts.uniqueKeys = enabled
	}
}

func New(opts ...MetadataOption) MetadataContainer {
	m := &metadata{
		Labels: make(Labels),
	}

	for _, o := range opts {
		o(m)
	}

	if m.opts.levelSeparator == "" {
		m.opts.levelSeparator = defaultLevelSeparator
	}

	return m
}

func (m *metadata) Level(name string, opts ...MetadataOption) MetadataContainer {
	inheritedOpts := []MetadataOption{
		withMetadataOpts(m.opts),
		WithPrefix(name),
		WithMetadata(m),
	}
	if m.mu != nil {
		opts = append(opts, WithConcurrencySupport())
	}

	return New(append(inheritedOpts, opts...)...)
}

func (m *metadata) GetLabels() Labels {
	m.rlock()
	defer m.runlock()

	return m.Labels
}

func (m *metadata) GetLabelsSlice() []SlicedLabel {
	m.rlock()
	defer m.runlock()

	labelsSlice := make([]SlicedLabel, 0)
	for name, values := range m.Labels {
		for _, value := range values {
			labelsSlice = append(labelsSlice, SlicedLabel{
				Name:  name,
				Value: value,
			})
		}
	}

	return labelsSlice
}

func (m *metadata) AddLabels(labels Labels) {
	m.lock()
	defer m.unlock()

	for k, v := range labels {
		m.AddLabel(k, v...)
	}
}

func (m *metadata) AddLabel(name string, values ...string) {
	m.lock()
	defer m.unlock()

	if m.opts.prefix != "" {
		if name != "" {
			name = strings.Join([]string{m.opts.prefix, m.opts.levelSeparator, name}, "")
		} else {
			name = m.opts.prefix
		}
	}

	if m.MetadataContainer != nil {
		m.MetadataContainer.AddLabel(name, values...)
		if !m.opts.storeOnSubLevel {
			return
		}
	}

	if _, ok := m.Labels[name]; !ok {
		m.Labels[name] = make([]string, 0)
	} else if m.opts.uniqueKeys {
		m.Labels[name] = values[len(values)-1:]
		return
	}

	m.Labels[name] = append(m.Labels[name], values...)

	if m.opts.uniqueValues {
		m.Labels[name] = m.unique(m.Labels[name])
	}
}

func (m *metadata) unique(values []string) []string {
	filteredValues := make([]string, 0)
	keys := make(map[string]struct{})

	for _, v := range values {
		if _, found := keys[v]; !found {
			keys[v] = struct{}{}
			filteredValues = append(filteredValues, v)
		}
	}

	return filteredValues
}

func (m *metadata) lock() {
	if m.mu != nil {
		m.mu.Lock()
	}
}

func (m *metadata) unlock() {
	if m.mu != nil {
		m.mu.Unlock()
	}
}

func (m *metadata) rlock() {
	if m.mu != nil {
		m.mu.RLock()
	}
}

func (m *metadata) runlock() {
	if m.mu != nil {
		m.mu.RUnlock()
	}
}

func ConvertMapStringToLabels(input map[string]string) Labels {
	labels := make(Labels)

	for k, v := range input {
		labels[k] = []string{v}
	}

	return labels
}
