package controller

import "k8s.io/apimachinery/pkg/labels"

// NamespaceFilter decides whether a namespace should be watched. A namespace
// is selected when its name is in the explicit list OR its labels match the
// label selector (OR / union semantics).
type NamespaceFilter struct {
	names    map[string]struct{}
	selector labels.Selector
}

// NewNamespaceFilter builds a filter from an explicit namespace list and a
// standard Kubernetes label selector string. An empty labelSelector means no
// selector; an invalid one returns an error.
func NewNamespaceFilter(namespaces []string, labelSelector string) (*NamespaceFilter, error) {
	f := &NamespaceFilter{names: make(map[string]struct{}, len(namespaces))}
	for _, ns := range namespaces {
		if ns != "" {
			f.names[ns] = struct{}{}
		}
	}
	if labelSelector != "" {
		sel, err := labels.Parse(labelSelector)
		if err != nil {
			return nil, err
		}
		f.selector = sel
	}
	return f, nil
}

// Enabled reports whether any namespace restriction is configured.
func (f *NamespaceFilter) Enabled() bool {
	return len(f.names) > 0 || f.selector != nil
}

// UsesLabelSelector reports whether a label selector is configured. When true
// the cache must run cluster-wide so labels can be evaluated dynamically.
func (f *NamespaceFilter) UsesLabelSelector() bool {
	return f.selector != nil
}

// Matches reports whether the namespace with the given name and labels should
// be watched.
func (f *NamespaceFilter) Matches(name string, nsLabels map[string]string) bool {
	if !f.Enabled() {
		return true
	}
	if _, ok := f.names[name]; ok {
		return true
	}
	return f.selector != nil && f.selector.Matches(labels.Set(nsLabels))
}
