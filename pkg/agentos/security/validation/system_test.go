// Package validation provides security validation for AgentOS
package validation

import (
	"strings"
	"testing"
)

func TestSystemNameValidator_Validate(t *testing.T) {
	validator := NewSystemNameValidator()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid names
		{"valid simple", "my-system", false},
		{"valid with underscore", "my_system", false},
		{"valid uppercase", "MySystem", false},
		{"valid alphanumeric", "system123", false},
		{"valid hyphen", "my-system-name", false},
		{"valid single letter", "a", false},
		{"valid with multiple hyphens", "my-long-system-name", false},
		{"valid with multiple underscores", "my_long_system_name", false},
		{"valid mixed", "My_System-123", false},

		// Invalid - empty
		{"empty", "", true},

		// Invalid - too long
		{"too long", strings.Repeat("a", 65), true},

		// Invalid - starts with number
		{"starts with number", "123system", true},
		{"starts with number 2", "1a", true},

		// Invalid - path traversal
		{"path traversal", "../../../etc/passwd", true},
		{"path traversal 2", "..\\windows\\system32", true},
		{"path traversal 3", "system/../other", true},

		// Invalid - null byte
		{"null byte", "system\x00hidden", true},

		// Invalid - hidden file
		{"hidden file", ".hidden", true},
		{"hidden file 2", ".system", true},

		// Invalid - reserved words
		{"reserved con", "con", true},
		{"reserved CON", "CON", true},
		{"reserved aux", "aux", true},
		{"reserved nul", "nul", true},
		{"reserved prn", "prn", true},
		{"reserved com1", "com1", true},
		{"reserved lpt1", "lpt1", true},
		{"reserved dot", ".", true},
		{"reserved dotdot", "..", true},

		// Invalid - special characters
		{"slash", "my/system", true},
		{"backslash", "my\\system", true},
		{"space", "my system", true},
		{"tab", "my\tsystem", true},
		{"newline", "my\nsystem", true},
		{"colon", "my:system", true},
		{"semicolon", "my;system", true},
		{"pipe", "my|system", true},
		{"asterisk", "my*system", true},
		{"question", "my?system", true},
		{"quote", `my"system`, true},
		{"single quote", "my'system", true},
		{"less than", "my<system", true},
		{"greater than", "my>system", true},
		{"ampersand", "my&system", true},
		{"dollar", "my$system", true},
		{"hash", "my#system", true},
		{"at", "my@system", true},
		{"exclamation", "my!system", true},
		{"percent", "my%system", true},
		{"caret", "my^system", true},
		{"plus", "my+system", true},
		{"equals", "my=system", true},
		{"brace open", "my{system", true},
		{"brace close", "my}system", true},
		{"bracket open", "my[system", true},
		{"bracket close", "my]system", true},
		{"paren open", "my(system", true},
		{"paren close", "my)system", true},
		{"backtick", "my`system", true},
		{"tilde", "my~system", true},
		{"comma", "my,system", true},

		// Edge cases
		{"edge 64 chars", strings.Repeat("a", 64), false},
		{"edge 63 chars", strings.Repeat("a", 63), false},
		{"only hyphen", "-", true},
		{"only underscore", "_", true},
		{"hyphen start", "-system", true},
		{"underscore start", "_system", true},
		{"ends hyphen", "system-", false},
		{"ends underscore", "system_", false},
		{"double hyphen", "my--system", false},
		{"double underscore", "my__system", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestSystemNameValidator_Sanitize(t *testing.T) {
	validator := NewSystemNameValidator()

	tests := []struct {
		input    string
		expected string
	}{
		// Simple cases
		{"MySystem", "mysystem"},
		{"my system", "my_system"},
		{"my-system", "my-system"},
		{"my_system", "my_system"},

		// Remove invalid chars
		{"my/system", "mysystem"},
		{"my\\system", "mysystem"},
		{"my:system", "mysystem"},
		{"my.system", "mysystem"},
		{"my,system", "mysystem"},
		{"my;system", "mysystem"},

		// Fix starting with number
		{"123system", "system"},
		{"1a2b3c", "abc"},

		// Handle reserved words
		{"con", "sys_con"},
		{"aux", "sys_aux"},
		{"nul", "sys_nul"},

		// Complex cases
		{"My System 123", "my_system_123"},
		{"System.Name-Here", "systemname-here"},
		{"   spaces   ", "spaces"},
		{"!!!special!!!", "special"},
		{"123-456-789", "sys_123-456-789"},

		// Edge cases
		{"", ""},
		{"...", ""},
		{"!!!", ""},
		{"123", "sys_123"},
		{"._-", "sys__"},

		// Length limit
		{strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := validator.Sanitize(tt.input)
			if result != tt.expected {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSystemNameValidator_ValidateAndSanitize(t *testing.T) {
	validator := NewSystemNameValidator()

	// Test that valid names pass through
	t.Run("valid name unchanged", func(t *testing.T) {
		input := "valid-system"
		result, err := validator.ValidateAndSanitize(input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != input {
			t.Errorf("valid name changed: got %q, want %q", result, input)
		}
	})

	// Test that invalid names are sanitized
	t.Run("invalid name sanitized", func(t *testing.T) {
		input := "My System!"
		result, err := validator.ValidateAndSanitize(input)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if result != "my_system" {
			t.Errorf("unexpected result: got %q, want %q", result, "my_system")
		}
	})

	// Test unfixable names
	t.Run("unfixable name", func(t *testing.T) {
		input := "!!!"
		result, err := validator.ValidateAndSanitize(input)
		if err == nil {
			t.Error("expected error for unfixable name")
		}
		if result != "" {
			t.Errorf("expected empty result, got %q", result)
		}
	})
}

func TestNewSystemNameValidatorWithLength(t *testing.T) {
	validator := NewSystemNameValidatorWithLength(10)

	tests := []struct {
		input   string
		wantErr bool
	}{
		{"short", false},
		{"exactlyten", false}, // 10 chars
		{"tooolong123", true}, // 11 chars
		{strings.Repeat("a", 10), false},
		{strings.Repeat("a", 11), true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := validator.Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestPathTraversalAttacks(t *testing.T) {
	validator := NewSystemNameValidator()

	attacks := []string{
		// Classic path traversal
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32\\config\\sam",
		"....//....//etc/passwd",
		"..%2f..%2f..%2fetc/passwd",
		"..%5c..%5cwindows%5csystem32",
		"%2e%2e%2f%2e%2e%2f%2e%2e%2fetc/passwd",

		// Absolute paths
		"/etc/passwd",
		"C:\\Windows\\System32",
		"/",
		"C:\\",

		// Encoded variations
		"%2e%2e%2f",
		"%252e%252e%252f",
		"..%c0%af",
		"..%ef%bc%8f",

		// Double encoding
		"%252e%252e%252f",
		"%25252e%25252e%25252f",

		// Unicode normalization
		"..\u2215..\u2215..\u2215etc/passwd",
		"..\uFF0F..\uFF0F..\uFF0Fetc/passwd",

		// Null byte injection
		"system\x00.txt",
		"file\x00.php",

		// Directory traversal with valid filename
		"../../../etc/passwd.system",
		"system/../../../etc/passwd",

		// Multiple slashes
		"..////..//..//etc/passwd",
		"..\\\\..\\\\..\\\\etc/passwd",

		// Current directory
		"./system",
		"system/./config",
		"system/../config",

		// Home directory
		"~/system",
		"~root/system",
		"~user/system",
	}

	for _, attack := range attacks {
		t.Run("attack_"+attack, func(t *testing.T) {
			err := validator.Validate(attack)
			if err == nil {
				t.Errorf("Path traversal attack %q should be rejected", attack)
			}
		})
	}
}

func TestXSSAndInjectionAttempts(t *testing.T) {
	validator := NewSystemNameValidator()

	attacks := []string{
		// XSS attempts
		"<script>alert('xss')</script>",
		"<img src=x onerror=alert('xss')>",
		"javascript:alert('xss')",
		"<svg onload=alert('xss')>",

		// SQL injection
		"system'; DROP TABLE systems; --",
		"system' OR '1'='1",
		"system' UNION SELECT * FROM users --",

		// Command injection
		"system; rm -rf /",
		"system && cat /etc/passwd",
		"system|whoami",
		"system`whoami`",
		"$(whoami)",

		// Template injection
		"{{7*7}}",
		"${7*7}",
		"#{7*7}",
	}

	for _, attack := range attacks {
		t.Run("attack", func(t *testing.T) {
			err := validator.Validate(attack)
			if err == nil {
				t.Errorf("Injection attempt %q should be rejected", attack)
			}
		})
	}
}

func BenchmarkValidate(b *testing.B) {
	validator := NewSystemNameValidator()
	names := []string{
		"valid-system",
		"my-system-name",
		"system123",
		"../../../etc/passwd",
		"con",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.Validate(names[i%len(names)])
	}
}

func BenchmarkSanitize(b *testing.B) {
	validator := NewSystemNameValidator()
	names := []string{
		"My System",
		"123-system",
		"../../../etc/passwd",
		"con",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		validator.Sanitize(names[i%len(names)])
	}
}
