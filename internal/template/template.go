// package template provides simple templating functionality for cloud-init scripts
package template

import (
	"fmt"
	"strings"
)

// Variable placeholder format
const (
	VariablePrefix = "{{%"
	VariableSuffix = "%}}"
)

// ValidationResult contains information about template validation
type ValidationResult struct {
	Valid            bool     // Whether the template is valid
	MissingVariables []string // Variables in template with no values provided
	UnusedVariables  []string // Variables provided but not used in template
}

// ExtractVariables finds all variable placeholders in the template
func ExtractVariables(template string) []string {
	var variables []string
	var unique = make(map[string]bool)

	// Find all instances of VariablePrefix followed by any chars followed by VariableSuffix
	startIdx := 0
	for {
		start := strings.Index(template[startIdx:], VariablePrefix)
		if start == -1 {
			break
		}
		start += startIdx
		end := strings.Index(template[start:], VariableSuffix)
		if end == -1 {
			break
		}
		end += start

		if end > start+len(VariablePrefix) {
			variable := template[start+len(VariablePrefix) : end]
			if !unique[variable] {
				unique[variable] = true
				variables = append(variables, variable)
			}
		}

		startIdx = end + len(VariableSuffix)
	}

	return variables
}

// ValidateTemplate checks if all variables in the template have values
// and all provided values are used in the template
func ValidateTemplate(template string, keyValues map[string]string) ValidationResult {
	result := ValidationResult{
		Valid:            true,
		MissingVariables: []string{},
		UnusedVariables:  []string{},
	}

	// Extract variables from template
	templateVars := ExtractVariables(template)

	// Find missing variables (in template but not in keyValues)
	for _, v := range templateVars {
		if _, exists := keyValues[v]; !exists {
			result.MissingVariables = append(result.MissingVariables, v)
			result.Valid = false
		}
	}

	// Find unused variables (in keyValues but not in template)
	templateVarMap := make(map[string]bool)
	for _, v := range templateVars {
		templateVarMap[v] = true
	}

	for k := range keyValues {
		if !templateVarMap[k] {
			result.UnusedVariables = append(result.UnusedVariables, k)
		}
	}

	return result
}

// ReplaceVariables replaces all occurrences of {{%key%}} in the provided
// template string with corresponding values from the keyValues map
func ReplaceVariables(template string, keyValues map[string]string) string {
	result := template
	for key, value := range keyValues {
		placeholder := fmt.Sprintf("%s%s%s", VariablePrefix, key, VariableSuffix)
		result = strings.Replace(result, placeholder, value, -1)
	}
	return result
}

// ParseKeyValueString parses a string of format "key:value,key:value" or "key:value","key:value" into a map
// It handles quoted values to allow commas within values and preserves colons in values
func ParseKeyValueString(input string) (map[string]string, error) {
	result := make(map[string]string)
	if input == "" {
		return result, nil
	}

	// First attempt to parse as CSV if we detect quotes
	if strings.Contains(input, "\"") {
		values, err := parseQuotedCSV(input)
		if err == nil {
			for _, pair := range values {
				if kv, err := parseKeyValue(pair); err == nil {
					result[kv.key] = kv.value
				} else {
					return nil, err
				}
			}
			return result, nil
		}
		// If parsing as CSV fails, fall through to comma-split approach
	}

	// Traditional comma-split approach
	pairs := strings.Split(input, ",")
	for _, pair := range pairs {
		kv, err := parseKeyValue(pair)
		if err != nil {
			return nil, err
		}
		result[kv.key] = kv.value
	}

	return result, nil
}

// keyValue is a helper struct for parseKeyValue
type keyValue struct {
	key   string
	value string
}

// parseKeyValue parses a single "key:value" string
func parseKeyValue(pair string) (keyValue, error) {
	parts := strings.SplitN(pair, ":", 2)
	if len(parts) != 2 {
		return keyValue{}, fmt.Errorf("invalid key-value pair format: %s (expected key:value)", pair)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	// Trim quotes if they exist
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
		(value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}

	if key == "" {
		return keyValue{}, fmt.Errorf("empty key in pair: %s", pair)
	}

	return keyValue{key: key, value: value}, nil
}

// parseQuotedCSV parses comma-separated values that may contain quotes
func parseQuotedCSV(input string) ([]string, error) {
	var result []string
	var currentField strings.Builder
	inQuotes := false

	for i := 0; i < len(input); i++ {
		char := input[i]

		switch {
		case char == '"':
			// Toggle quote state
			inQuotes = !inQuotes
		case char == ',' && !inQuotes:
			// End of field
			result = append(result, currentField.String())
			currentField.Reset()
		default:
			currentField.WriteByte(char)
		}
	}

	// Add the last field
	if currentField.Len() > 0 {
		result = append(result, currentField.String())
	}

	return result, nil
}
