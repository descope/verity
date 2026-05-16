package chartgen

import "testing"

// TestDecideEmptyMappingsAction documents the decision matrix for the
// post-mappings branch in processChart. The four outcomes cover:
//
//   - emitNormal: mappings exist; standard path
//   - emitChartValuesOnly: gitea-style — no mappings but chartValues exist
//   - emitPassthrough: victoria-logs-single-style — every discovered image
//     was intentionally excluded (unpatchableImages / exclude-names) so the
//     wrapper still ships but with no overrides
//   - emitSkip: genuine pipeline gap or empty chart; strict mode should
//     catch it. Includes the subtle case where SOME images were
//     discovered but not all were intentionally excluded — meaning at
//     least one image failed crane lookup against the patched registry.
func TestDecideEmptyMappingsAction(t *testing.T) {
	cases := []struct {
		name           string
		mappings       int
		chartValues    int
		imageRefs      int
		excluded       int
		want           emptyMappingsAction
		whyForFutureMe string
	}{
		{
			name: "mappings-present", mappings: 1, chartValues: 0, imageRefs: 5, excluded: 0, want: emitNormal,
			whyForFutureMe: "non-empty mappings always wins",
		},
		{
			name: "mappings-and-chartvalues", mappings: 2, chartValues: 3, imageRefs: 5, excluded: 0, want: emitNormal,
			whyForFutureMe: "mappings win even when chartValues exist",
		},
		{
			name: "gitea-style-chartvalues-only", mappings: 0, chartValues: 4, imageRefs: 2, excluded: 2, want: emitChartValuesOnly,
			whyForFutureMe: "gitea: 2 images both excluded, but chartValues carry the wrapper",
		},
		{
			name: "chartvalues-only-no-images", mappings: 0, chartValues: 1, imageRefs: 0, excluded: 0, want: emitChartValuesOnly,
			whyForFutureMe: "chartValues precedence: emit even when no images at all",
		},
		{
			name: "victoria-logs-single-passthrough", mappings: 0, chartValues: 0, imageRefs: 1, excluded: 1, want: emitPassthrough,
			whyForFutureMe: "all discovered images intentionally excluded — ship upstream chart unchanged",
		},
		{
			name: "passthrough-many-images-all-excluded", mappings: 0, chartValues: 0, imageRefs: 7, excluded: 7, want: emitPassthrough,
			whyForFutureMe: "every image hit exclude-names/unpatchableImages",
		},
		{
			name: "partial-exclusion-real-gap", mappings: 0, chartValues: 0, imageRefs: 3, excluded: 1, want: emitSkip,
			whyForFutureMe: "2 images survived to crane lookup but produced 0 mappings — real pipeline gap, NOT passthrough",
		},
		{
			name: "crane-lookup-gap-no-exclusions", mappings: 0, chartValues: 0, imageRefs: 5, excluded: 0, want: emitSkip,
			whyForFutureMe: "5 images all reached crane lookup, none patched — strict mode must catch this",
		},
		{
			name: "truly-empty-skip", mappings: 0, chartValues: 0, imageRefs: 0, excluded: 0, want: emitSkip,
			whyForFutureMe: "no images, no chartValues — genuine config gap",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideEmptyMappingsAction(tc.mappings, tc.chartValues, tc.imageRefs, tc.excluded)
			if got != tc.want {
				t.Errorf("decideEmptyMappingsAction(m=%d,cv=%d,ir=%d,ex=%d) = %d, want %d (%s)",
					tc.mappings, tc.chartValues, tc.imageRefs, tc.excluded, got, tc.want, tc.whyForFutureMe)
			}
		})
	}
}
