package metadatax

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
)

const (
	defaultSegmentSeparator = ":"
)

type MetadataContainer interface {
	MetadataLabels
}

type MetadataLabels interface {
	GetLabels() Labels
	GetLabelValue(name string) string
	GetLabelValues(name string) ([]string, bool)
	GetLabelsSlice() []SlicedLabel
	AddLabel(name string, value ...string) MetadataContainer
	AddLabels(Labels) MetadataContainer
	Segment(name string, opts ...MetadataOption) MetadataContainer
	String() string
}

type Labels map[string][]string

type SlicedLabel struct {
	Name  string
	Value string
}

type MetadataOption func(*metadata)

type metadataOpts struct {
	prefix           string
	segmentSeparator string
	storeAtSegment   bool
	uniqueValues     bool
	uniqueKeys       bool
	allowEmptyValues bool
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

func WithSegmentSeparator(separator string) MetadataOption {
	return func(m *metadata) {
		m.opts.segmentSeparator = separator
	}
}

func WithStoreAtSegment(store bool) MetadataOption {
	return func(m *metadata) {
		m.opts.storeAtSegment = store
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

func WithAllowEmptyValues(enabled bool) MetadataOption {
	return func(m *metadata) {
		m.opts.allowEmptyValues = enabled
	}
}

func New(opts ...MetadataOption) MetadataContainer {
	m := &metadata{
		Labels: make(Labels),
	}

	for _, o := range opts {
		o(m)
	}

	if m.opts.segmentSeparator == "" {
		m.opts.segmentSeparator = defaultSegmentSeparator
	}

	return m
}

func (m *metadata) Segment(name string, opts ...MetadataOption) MetadataContainer {
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

func (m *metadata) GetLabelValue(name string) string {
	m.rlock()
	defer m.runlock()

	s, ok := m.Labels[name]
	if ok && len(s) > 0 {
		return s[0]
	}

	return ""
}

func (m *metadata) GetLabelValues(name string) ([]string, bool) {
	m.rlock()
	defer m.runlock()

	s, ok := m.Labels[name]

	return s, ok
}

func (m *metadata) GetLabels() Labels {
	m.rlock()
	defer m.runlock()

	return m.Labels
}

func (m *metadata) String() string {
	var output string

	slice := slices.Clone(m.GetLabelsSlice())
	sort.SliceStable(slice, func(i, j int) bool {
		if strings.Compare(slice[i].Name, slice[j].Name) < 0 {
			return true
		}

		return false
	})

	for _, l := range m.GetLabelsSlice() {
		output += fmt.Sprintf("%s=%s\n", l.Name, l.Value)
	}

	return output
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

	sort.SliceStable(labelsSlice, func(i, j int) bool {
		return strings.Compare(labelsSlice[i].Name, labelsSlice[j].Name) < 0
	})

	return labelsSlice
}

func (m *metadata) AddLabels(labels Labels) MetadataContainer {
	m.lock()
	defer m.unlock()

	for k, v := range labels {
		m.AddLabel(k, v...)
	}

	return m
}

func (m *metadata) AddLabel(name string, values ...string) MetadataContainer {
	m.lock()
	defer m.unlock()

	if !m.opts.allowEmptyValues {
		if values == nil {
			return m
		}
		_values := values[:0]
		for _, v := range values {
			if v == "" {
				continue
			}
			_values = append(_values, v)
		}
		values = _values
		if len(values) == 0 {
			return m
		}
	}

	if m.opts.prefix != "" {
		if name != "" {
			name = strings.Join([]string{m.opts.prefix, m.opts.segmentSeparator, name}, "")
		} else {
			name = m.opts.prefix
		}
	}

	if m.MetadataContainer != nil {
		m.MetadataContainer.AddLabel(name, values...)
		if !m.opts.storeAtSegment {
			return m
		}
	}

	if _, ok := m.Labels[name]; !ok {
		m.Labels[name] = make([]string, 0)
	} else if m.opts.uniqueKeys {
		m.Labels[name] = values[len(values)-1:]
		return m
	}

	m.Labels[name] = append(m.Labels[name], values...)

	if m.opts.uniqueValues {
		m.Labels[name] = m.unique(m.Labels[name])
	}

	return m
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
