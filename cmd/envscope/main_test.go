// Copyright 2026 Stanislav Senotrusov
//
// This work is dual-licensed under the Apache License, Version 2.0
// and the MIT License. Refer to the LICENSE file in the top-level directory
// for the full license terms.
//
// SPDX-License-Identifier: Apache-2.0 OR MIT

package main

import (
	"testing"
)

func TestIsValidVarName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"VALID_VAR", true},
		{"_VALID", true},
		{"validVar123", true},
		{"1INVALID", false},
		{"INV-ALID", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isValidVarName(tt.name); got != tt.expected {
			t.Errorf("isValidVarName(%q) = %v; want %v", tt.name, got, tt.expected)
		}
	}
}

func TestIsSubPath(t *testing.T) {
	tests := []struct {
		parent   string
		child    string
		expected bool
	}{
		{"/", "/home/user", true},
		{"/home/user", "/home/user/foo", true},
		{"/home/user", "/home/user", false},
		{"/home/user", "/home/user2", false},
		{"/home/*", "/home/user/foo", true},
		{"/home/*/src", "/home/user/src/foo", true},
	}

	for _, tt := range tests {
		if got := isSubPath(tt.parent, tt.child); got != tt.expected {
			t.Errorf("isSubPath(%q, %q) = %v; want %v", tt.parent, tt.child, got, tt.expected)
		}
	}
}

func TestExpandTilde(t *testing.T) {
	tests := []struct {
		val      string
		isPath   bool
		expected string
	}{
		{"~", false, "/home/user"},
		{"~/foo", false, "/home/user/foo"},
		{"a~/foo", false, "a~/foo"},
		{"~/foo:~/bar", true, "/home/user/foo:/home/user/bar"},
		{":/bin:~/foo", true, ":/bin:/home/user/foo"},
	}

	for _, tt := range tests {
		if got := expandTilde(tt.val, "/home/user", tt.isPath); got != tt.expected {
			t.Errorf("expandTilde(%q, isPath=%v) = %q; want %q", tt.val, tt.isPath, got, tt.expected)
		}
	}
}

func TestParseVarLine(t *testing.T) {
	var currentVars []EnvVar
	var allVars []string
	seenVars := make(map[string]bool)

	// Validate basic variable
	err := parseVarLine("FOO=bar", "/home/user", &currentVars, &allVars, seenVars)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(currentVars) != 1 || currentVars[0].Name != "FOO" || currentVars[0].Value != "bar" || currentVars[0].Cache {
		t.Fatalf("parsed wrongly: %+v", currentVars)
	}

	// Validate dynamic caching fallback (handles hashes securely)
	currentVars = nil
	err = parseVarLine("BAR=$(echo \"foo#bar\") # cache", "/home/user", &currentVars, &allVars, seenVars)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(currentVars) != 1 || currentVars[0].Name != "BAR" || currentVars[0].Value != "echo \"foo#bar\"" || !currentVars[0].Cache {
		t.Fatalf("parsed wrongly: %+v", currentVars)
	}

	// Double quotes rejected
	err = parseVarLine("REJECT=\"quotes\"", "/home/user", &currentVars, &allVars, seenVars)
	if err == nil {
		t.Fatalf("expected error on quotes, got nil")
	}
}
