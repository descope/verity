package cmd

import "testing"

func TestLatestPatchedTagFromList(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		sourceTag string
		want      string
	}{
		{
			name:      "no patched tags",
			tags:      []string{"1.29.3", "1.29.4"},
			sourceTag: "1.29.3",
			want:      "",
		},
		{
			name:      "bare patched only",
			tags:      []string{"1.29.3", "1.29.3-patched"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched",
		},
		{
			name:      "numbered beats bare",
			tags:      []string{"1.29.3-patched", "1.29.3-patched-2"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-2",
		},
		{
			name:      "highest number wins",
			tags:      []string{"1.29.3-patched", "1.29.3-patched-2", "1.29.3-patched-3"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-3",
		},
		{
			name:      "unrelated tags ignored",
			tags:      []string{"1.29.3-patched-3", "1.29.4-patched-2", "1.29.3-something"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-3",
		},
		{
			name:      "tag with v prefix",
			tags:      []string{"v8.2.2-patched", "v8.2.2-patched-2"},
			sourceTag: "v8.2.2",
			want:      "v8.2.2-patched-2",
		},
		{
			name:      "tag with dots in version is not confused by similar prefix",
			tags:      []string{"1.29.3-patched-2", "1.29.30-patched-5"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-2",
		},
		{
			name:      "highest numbered wins among explicit versions",
			tags:      []string{"1.29.3-patched-1", "1.29.3-patched-2"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-2",
		},
		{
			name:      "explicit -patched-1 beats bare -patched",
			tags:      []string{"2.5.0-patched", "2.5.0-patched-1"},
			sourceTag: "2.5.0",
			want:      "2.5.0-patched-1",
		},
		{
			name:      "unparseable patch number skipped",
			tags:      []string{"1.29.3-patched", "1.29.3-patched-99999999999999999999"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := latestPatchedTagFromList(tc.tags, tc.sourceTag)
			if got != tc.want {
				t.Errorf("latestPatchedTagFromList(%v, %q) = %q, want %q", tc.tags, tc.sourceTag, got, tc.want)
			}
		})
	}
}

func TestNextPatchedTag(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		sourceTag string
		want      string
	}{
		{
			name:      "no existing patched tags → bare -patched",
			tags:      []string{"1.29.3", "1.29.4"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched",
		},
		{
			name:      "bare -patched exists → -patched-1",
			tags:      []string{"1.29.3", "1.29.3-patched"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-1",
		},
		{
			name:      "bare -patched and -patched-1 exist → -patched-2",
			tags:      []string{"1.29.3-patched", "1.29.3-patched-1"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-2",
		},
		{
			name:      "highest numbered -patched-N incremented",
			tags:      []string{"1.29.3-patched", "1.29.3-patched-2", "1.29.3-patched-3"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-4",
		},
		{
			name:      "unrelated tags do not affect result",
			tags:      []string{"1.29.3-patched-3", "1.29.4-patched-99", "1.29.3-other"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-4",
		},
		{
			name:      "tag with v prefix",
			tags:      []string{"v8.2.2-patched", "v8.2.2-patched-1"},
			sourceTag: "v8.2.2",
			want:      "v8.2.2-patched-2",
		},
		{
			name:      "dots in version are not confused by similar prefix",
			tags:      []string{"1.29.3-patched-2", "1.29.30-patched-5"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-3",
		},
		{
			name:      "unparseable patch number is skipped, bare wins",
			tags:      []string{"1.29.3-patched", "1.29.3-patched-99999999999999999999"},
			sourceTag: "1.29.3",
			want:      "1.29.3-patched-1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextPatchedTag(tc.tags, tc.sourceTag)
			if got != tc.want {
				t.Errorf("nextPatchedTag(%v, %q) = %q, want %q", tc.tags, tc.sourceTag, got, tc.want)
			}
		})
	}
}
