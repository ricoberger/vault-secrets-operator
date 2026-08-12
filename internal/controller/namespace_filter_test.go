package controller

import "testing"

func TestNamespaceFilter(t *testing.T) {
	tests := []struct {
		name          string
		namespaces    []string
		labelSelector string
		wantErr       bool
		enabled       bool
		usesSelector  bool
		matchName     string
		matchLabels   map[string]string
		wantMatch     bool
	}{
		{
			name:      "disabled matches everything",
			enabled:   false,
			matchName: "anything",
			wantMatch: true,
		},
		{
			name:       "explicit name in list matches",
			namespaces: []string{"team-a", "team-b"},
			enabled:    true,
			matchName:  "team-a",
			wantMatch:  true,
		},
		{
			name:       "explicit name not in list does not match",
			namespaces: []string{"team-a"},
			enabled:    true,
			matchName:  "team-c",
			wantMatch:  false,
		},
		{
			name:          "label selector matches",
			labelSelector: "watch=true",
			enabled:       true,
			usesSelector:  true,
			matchName:     "team-x",
			matchLabels:   map[string]string{"watch": "true"},
			wantMatch:     true,
		},
		{
			name:          "label selector does not match",
			labelSelector: "watch=true",
			enabled:       true,
			usesSelector:  true,
			matchName:     "team-x",
			matchLabels:   map[string]string{"watch": "false"},
			wantMatch:     false,
		},
		{
			name:          "OR union: name matches even when label does not",
			namespaces:    []string{"team-a"},
			labelSelector: "watch=true",
			enabled:       true,
			usesSelector:  true,
			matchName:     "team-a",
			matchLabels:   map[string]string{"watch": "false"},
			wantMatch:     true,
		},
		{
			name:          "OR union: label matches even when name is absent",
			namespaces:    []string{"team-a"},
			labelSelector: "watch=true",
			enabled:       true,
			usesSelector:  true,
			matchName:     "team-z",
			matchLabels:   map[string]string{"watch": "true"},
			wantMatch:     true,
		},
		{
			name:          "invalid selector errors",
			labelSelector: "!!!bad",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := NewNamespaceFilter(tt.namespaces, tt.labelSelector)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := f.Enabled(); got != tt.enabled {
				t.Errorf("Enabled() = %v, want %v", got, tt.enabled)
			}
			if got := f.UsesLabelSelector(); got != tt.usesSelector {
				t.Errorf("UsesLabelSelector() = %v, want %v", got, tt.usesSelector)
			}
			if got := f.Matches(tt.matchName, tt.matchLabels); got != tt.wantMatch {
				t.Errorf("Matches(%q, %v) = %v, want %v", tt.matchName, tt.matchLabels, got, tt.wantMatch)
			}
		})
	}
}
