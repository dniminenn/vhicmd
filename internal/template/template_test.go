package template

import (
	"reflect"
	"sort"
	"testing"
)

func TestExtractVariables(t *testing.T) {
	tests := []struct {
		name     string
		template string
		expected []string
	}{
		{
			name:     "simple variables",
			template: "Hello {{%name%}}, your ID is {{%id%}}",
			expected: []string{"name", "id"},
		},
		{
			name:     "repeated variables",
			template: "{{%greeting%}} {{%name%}}! {{%greeting%}} again, {{%name%}}!",
			expected: []string{"greeting", "name"},
		},
		{
			name:     "no variables",
			template: "Hello world!",
			expected: []string{},
		},
		{
			name:     "incomplete placeholder",
			template: "Hello {{%name",
			expected: []string{},
		},
		{
			name:     "cloudinit example",
			template: "#cloud-config\nhostname: {{%hostname%}}\nusers:\n  - name: {{%username%}}\n    passwd: {{%password%}}",
			expected: []string{"hostname", "username", "password"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractVariables(tt.template)

			// Sort both slices for comparison
			sort.Strings(result)
			sort.Strings(tt.expected)

			// Check lengths first
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractVariables() = %v (len %d), want %v (len %d)",
					result, len(result), tt.expected, len(tt.expected))
				return
			}

			// For empty slices, just return if both are empty
			if len(result) == 0 && len(tt.expected) == 0 {
				return
			}

			// Otherwise, compare elements
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ExtractVariables() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		vars        map[string]string
		wantValid   bool
		wantMissing []string
		wantUnused  []string
	}{
		{
			name:        "all variables provided",
			template:    "Hello {{%name%}}, your ID is {{%id%}}",
			vars:        map[string]string{"name": "World", "id": "12345"},
			wantValid:   true,
			wantMissing: []string{},
			wantUnused:  []string{},
		},
		{
			name:        "missing variables",
			template:    "Hello {{%name%}}, your ID is {{%id%}}",
			vars:        map[string]string{"name": "World"},
			wantValid:   false,
			wantMissing: []string{"id"},
			wantUnused:  []string{},
		},
		{
			name:        "unused variables",
			template:    "Hello {{%name%}}!",
			vars:        map[string]string{"name": "World", "id": "12345", "extra": "unused"},
			wantValid:   true, // Unused variables don't invalidate the template
			wantMissing: []string{},
			wantUnused:  []string{"id", "extra"},
		},
		{
			name:        "both missing and unused",
			template:    "Hello {{%name%}}, your email is {{%email%}}",
			vars:        map[string]string{"name": "World", "id": "12345"},
			wantValid:   false,
			wantMissing: []string{"email"},
			wantUnused:  []string{"id"},
		},
		{
			name:        "no variables",
			template:    "Hello world!",
			vars:        map[string]string{},
			wantValid:   true,
			wantMissing: []string{},
			wantUnused:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateTemplate(tt.template, tt.vars)

			if result.Valid != tt.wantValid {
				t.Errorf("ValidateTemplate().Valid = %v, want %v", result.Valid, tt.wantValid)
			}

			sort.Strings(result.MissingVariables)
			sort.Strings(tt.wantMissing)
			sort.Strings(result.UnusedVariables)
			sort.Strings(tt.wantUnused)

			if !reflect.DeepEqual(result.MissingVariables, tt.wantMissing) {
				t.Errorf("ValidateTemplate().MissingVariables = %v, want %v", result.MissingVariables, tt.wantMissing)
			}

			if !reflect.DeepEqual(result.UnusedVariables, tt.wantUnused) {
				t.Errorf("ValidateTemplate().UnusedVariables = %v, want %v", result.UnusedVariables, tt.wantUnused)
			}
		})
	}
}

func TestReplaceVariables(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]string
		expected string
	}{
		{
			name:     "simple replacement",
			template: "Hello {{%name%}}!",
			vars:     map[string]string{"name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "multiple replacements",
			template: "{{%greeting%}} {{%name%}}! Your ID is {{%id%}}.",
			vars:     map[string]string{"greeting": "Hello", "name": "User", "id": "12345"},
			expected: "Hello User! Your ID is 12345.",
		},
		{
			name:     "missing variable",
			template: "Hello {{%name%}}!",
			vars:     map[string]string{"username": "World"},
			expected: "Hello {{%name%}}!",
		},
		{
			name:     "empty input",
			template: "",
			vars:     map[string]string{"name": "World"},
			expected: "",
		},
		{
			name:     "cloudinit example",
			template: "#cloud-config\nhostname: {{%hostname%}}\nusers:\n  - name: {{%username%}}\n    passwd: {{%password%}}",
			vars:     map[string]string{"hostname": "test-vm", "username": "admin", "password": "securepass"},
			expected: "#cloud-config\nhostname: test-vm\nusers:\n  - name: admin\n    passwd: securepass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplaceVariables(tt.template, tt.vars)
			if result != tt.expected {
				t.Errorf("ReplaceVariables() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseKeyValueString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
		wantErr  bool
	}{
		{
			name:     "simple key-values",
			input:    "key1:value1,key2:value2",
			expected: map[string]string{"key1": "value1", "key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "empty input",
			input:    "",
			expected: map[string]string{},
			wantErr:  false,
		},
		{
			name:     "spaces in values",
			input:    "name:John Doe,job:Software Engineer",
			expected: map[string]string{"name": "John Doe", "job": "Software Engineer"},
			wantErr:  false,
		},
		{
			name:     "spaces around separators",
			input:    "key1 : value1 , key2 : value2",
			expected: map[string]string{"key1": "value1", "key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "quoted values with commas",
			input:    "name:\"John, Doe\",job:\"Software, Engineer\"",
			expected: map[string]string{"name": "John, Doe", "job": "Software, Engineer"},
			wantErr:  false,
		},
		{
			name:     "double-quoted format",
			input:    "\"key1:value1\",\"key2:value2\"",
			expected: map[string]string{"key1": "value1", "key2": "value2"},
			wantErr:  false,
		},
		{
			name:     "mixed formats",
			input:    "key1:value1,\"key2:value with, comma\"",
			expected: map[string]string{"key1": "value1", "key2": "value with, comma"},
			wantErr:  false,
		},
		{
			name:     "colons in values",
			input:    "time:10:30,key:value",
			expected: map[string]string{"time": "10:30", "key": "value"},
			wantErr:  false,
		},
		{
			name:     "complex example with SSH key",
			input:    "hostname:server1,ssh_key:\"ssh-rsa AAAAB3NzaC1yc2EAAA...\"",
			expected: map[string]string{"hostname": "server1", "ssh_key": "ssh-rsa AAAAB3NzaC1yc2EAAA..."},
			wantErr:  false,
		},
		{
			name:     "invalid format - missing value",
			input:    "key1:value1,key2",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "invalid format - empty key",
			input:    "key1:value1,:value2",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseKeyValueString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseKeyValueString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseKeyValueString() = %v, want %v", result, tt.expected)
			}
		})
	}
}
