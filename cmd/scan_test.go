package cmd

import "testing"

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"docker.io/library/nginx:1.27", "docker.io_library_nginx_1.27"},
		{"ghcr.io/verity-org/node:22", "ghcr.io_verity-org_node_22"},
		{"simple", "simple"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizeFilename(tt.input); got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
