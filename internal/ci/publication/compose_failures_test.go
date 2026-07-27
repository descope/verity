package publication

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompose_rejects_missing_duplicate_conflicting_and_undeclared_producers(t *testing.T) {
	integerData := composeIntegerManifest(t)
	chartData := composeChartManifest("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	tests := []struct {
		name    string
		mutate  func(*ComposeRequest)
		wantErr error
	}{
		{name: "missing charts", mutate: func(r *ComposeRequest) { r.Producers = r.Producers[:1] }, wantErr: ErrProducerMissing},
		{name: "duplicate charts", mutate: func(r *ComposeRequest) { r.Producers = append(r.Producers, r.Producers[1]) }, wantErr: ErrProducerDuplicate},
		{name: "conflicting source", mutate: func(r *ComposeRequest) {
			r.Producers[1].Data = composeChartManifest("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
		}, wantErr: ErrProducerIdentity},
		{name: "undeclared producer", mutate: func(r *ComposeRequest) { r.Producers[1].Name = "attacker" }, wantErr: ErrProducerUndeclared},
		{name: "conflicting artifact", mutate: func(r *ComposeRequest) { r.Producers[1].ArtifactName = "other" }, wantErr: ErrProducerConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given one malformed producer set.
			request := composeRequest(integerData, chartData)
			test.mutate(&request)

			// When it crosses the composition boundary.
			_, err := Compose(context.Background(), &request)

			// Then the typed discrepancy fails closed.
			require.ErrorIs(t, err, test.wantErr)
		})
	}
}
