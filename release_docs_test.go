package main

import (
	"os"
	"strings"
	"testing"
)

func TestReadmeDoesNotContainInternalReleaseReferences(t *testing.T) {
	content, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	readme := string(content)
	for _, forbidden := range []string{
		"groupme-backend",
		"-n apps",
		"quazmoz/quazmoz:groupme",
	} {
		if strings.Contains(readme, forbidden) {
			t.Fatalf("README.md should not contain %q", forbidden)
		}
	}
}
