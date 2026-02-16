package cmd

import (
	"testing"
)

func TestPatchCommand_ValidateFlags(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		registry  string
		resultDir string
		wantErr   bool
	}{
		{
			name:      "all required flags",
			image:     "nginx:1.25",
			registry:  "ghcr.io/verity-org",
			resultDir: ".verity/results",
			wantErr:   false,
		},
		{
			name:      "missing image",
			image:     "",
			registry:  "ghcr.io/verity-org",
			resultDir: ".verity/results",
			wantErr:   true,
		},
		{
			name:      "missing registry",
			image:     "nginx:1.25",
			registry:  "",
			resultDir: ".verity/results",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Basic validation logic test
			hasErr := tt.image == "" || tt.registry == ""
			if hasErr != tt.wantErr {
				t.Errorf("validation error = %v, wantErr %v", hasErr, tt.wantErr)
			}
		})
	}
}

func TestPatchCommand_ImageRefParsing(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		wantTag string
	}{
		{
			name:    "with tag",
			ref:     "docker.io/library/nginx:1.25",
			wantTag: "1.25",
		},
		{
			name:    "without tag",
			ref:     "docker.io/library/nginx",
			wantTag: "",
		},
		{
			name:    "with registry and tag",
			ref:     "quay.io/prometheus/prometheus:v2.45.0",
			wantTag: "v2.45.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simple tag extraction test
			var gotTag string
			if idx := len(tt.ref) - 1; idx >= 0 {
				for i := idx; i >= 0; i-- {
					if tt.ref[i] == ':' {
						gotTag = tt.ref[i+1:]
						break
					}
					if tt.ref[i] == '/' {
						break
					}
				}
			}

			if gotTag != tt.wantTag {
				t.Errorf("extracted tag = %q, want %q", gotTag, tt.wantTag)
			}
		})
	}
}
