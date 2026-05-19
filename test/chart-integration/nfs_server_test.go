//go:build integration

package integration

import "testing"

func TestNeedsNFSServer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		filter string
		want   bool
	}{
		{name: "full suite", filter: "", want: true},
		{name: "nfs shard", filter: nfsSubdirChartName, want: true},
		{name: "trimmed nfs shard", filter: "  " + nfsSubdirChartName + "  ", want: true},
		{name: "unrelated shard", filter: "falco", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := needsNFSServer(tc.filter); got != tc.want {
				t.Fatalf("needsNFSServer(%q)=%v want %v", tc.filter, got, tc.want)
			}
		})
	}
}
