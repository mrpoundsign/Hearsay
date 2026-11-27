package main

import (
	"testing"
)

func TestBuildAppSpec(t *testing.T) {
	spec := buildAppSpec()
	if spec.AppName != "Hearsay" {
		t.Errorf("expected AppName 'Hearsay', got %q", spec.AppName)
	}
	if spec.AppID != "com.github.mrpoundsign.hearsay" {
		t.Errorf("expected AppID 'com.github.mrpoundsign.hearsay', got %q", spec.AppID)
	}
	if spec.BinaryName != "Hearsay.exe" {
		t.Errorf("expected BinaryName 'Hearsay.exe', got %q", spec.BinaryName)
	}
	if got := spec.FileRenames["hearsay-core.exe"]; got != "Hearsay.exe" {
		t.Errorf("expected FileRenames['hearsay-core.exe'] == 'Hearsay.exe', got %q", got)
	}
	if len(spec.OptionalComponents) != 2 {
		t.Fatalf("expected 2 optional components, got %d", len(spec.OptionalComponents))
	}

	seenIDs := make(map[string]bool, len(spec.OptionalComponents))
	for _, comp := range spec.OptionalComponents {
		seenIDs[comp.ID] = true
		if comp.IsInstalled == nil {
			t.Errorf("component %q missing IsInstalled check", comp.ID)
			continue
		}
		if comp.IsInstalled(t.TempDir()) {
			t.Errorf("expected component %q IsInstalled to return false during tests", comp.ID)
		}
	}
	if !seenIDs["supertonic"] || !seenIDs["kokoro"] {
		t.Errorf("expected supertonic and kokoro components, got %+v", seenIDs)
	}
}
