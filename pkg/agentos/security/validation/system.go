// Package validation provides security validation for AgentOS
package validation

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// validSystemNamePattern matches valid system names
	// Must start with letter, contain only alphanumeric, hyphens, underscores
	// Max 64 characters
	validSystemNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

	// reservedWords contains system-reserved names (cross-platform)
	reservedWords = map[string]bool{
		"aux": true, "con": true, "nul": true, "prn": true,
		"com1": true, "com2": true, "com3": true, "com4": true,
		"com5": true, "com6": true, "com7": true, "com8": true, "com9": true,
		"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true,
		"lpt5": true, "lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
		"":     true,
		".":    true,
		"..":   true,
	}

	// dangerousPatterns contains patterns that could indicate path traversal
	dangerousPatterns = []string{
		"..", "...", "....",
		"~", "~root", "~user",
	}
)

// SystemNameValidator validates system names for security
type SystemNameValidator struct {
	maxLength int
}

// NewSystemNameValidator creates a new validator with defaults
func NewSystemNameValidator() *SystemNameValidator {
	return &SystemNameValidator{maxLength: 64}
}

// NewSystemNameValidatorWithLength creates a validator with custom max length
func NewSystemNameValidatorWithLength(maxLen int) *SystemNameValidator {
	return &SystemNameValidator{maxLength: maxLen}
}

// Validate checks if a system name is valid and secure
func (v *SystemNameValidator) Validate(name string) error {
	if name == "" {
		return fmt.Errorf("system name cannot be empty")
	}

	if len(name) > v.maxLength {
		return fmt.Errorf("system name too long: %d characters (max %d)", len(name), v.maxLength)
	}

	// Check for null bytes (null byte injection)
	if strings.ContainsRune(name, '\x00') {
		return fmt.Errorf("system name cannot contain null bytes")
	}

	// Check for path separators
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("system name cannot contain path separators (/ or \\)")
	}

	// Check for hidden file pattern
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("system name cannot start with a dot")
	}

	// Check for dangerous patterns
	lowerName := strings.ToLower(name)
	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerName, pattern) {
			return fmt.Errorf("system name contains dangerous pattern: %s", pattern)
		}
	}

	// Check against reserved words
	if reservedWords[lowerName] {
		return fmt.Errorf("system name '%s' is reserved", name)
	}

	// Validate pattern (alphanumeric, hyphens, underscores)
	if !validSystemNamePattern.MatchString(name) {
		return fmt.Errorf("system name must start with a letter and contain only alphanumeric characters, hyphens, and underscores")
	}

	return nil
}

// Sanitize attempts to clean and normalize a system name
// Returns empty string if sanitization is not possible
func (v *SystemNameValidator) Sanitize(name string) string {
	if name == "" {
		return ""
	}

	// Convert to lowercase
	result := strings.ToLower(name)

	// Replace spaces with underscores
	result = strings.ReplaceAll(result, " ", "_")

	// Remove invalid characters
	var sb strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		}
	}
	result = sb.String()

	// Remove leading digits
	for len(result) > 0 && result[0] >= '0' && result[0] <= '9' {
		result = result[1:]
	}

	// Check if empty after sanitization
	if result == "" {
		return ""
	}

	// Check against reserved words
	if reservedWords[result] {
		// Add prefix to avoid reserved word
		result = "sys_" + result
	}

	// Limit length
	if len(result) > v.maxLength {
		result = result[:v.maxLength]
	}

	// Remove trailing dash or underscore
	result = strings.TrimRight(result, "-_")

	return result
}

// ValidateAndSanitize attempts to validate, and if that fails, tries to sanitize
// Returns the sanitized name and any error
func (v *SystemNameValidator) ValidateAndSanitize(name string) (string, error) {
	// First try validation
	if err := v.Validate(name); err == nil {
		return name, nil
	}

	// Try sanitization
	sanitized := v.Sanitize(name)
	if sanitized == "" {
		return "", fmt.Errorf("cannot sanitize system name: %s", name)
	}

	// Validate the sanitized name
	if err := v.Validate(sanitized); err != nil {
		return "", fmt.Errorf("sanitized name is still invalid: %v", err)
	}

	return sanitized, nil
}

// IsReserved checks if a name is reserved
func IsReserved(name string) bool {
	return reservedWords[strings.ToLower(name)]
}
