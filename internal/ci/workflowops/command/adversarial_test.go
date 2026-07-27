package command

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func TestWorkflowOpsCLI_rejects_hostile_Integer_report_sets(t *testing.T) {
	tests := []struct {
		name    string
		plan    string
		reports []string
		result  string
	}{
		{
			name: "valid plus spoofed",
			plan: `[{"name":"alpha","version":"1","type":"default"}]`,
			reports: []string{
				exactCommandReport("alpha", "1", "42-1"),
				renderCommandReport(&commandReportFixture{
					image: "alpha", version: "1", runID: "999", attempt: 7,
					source: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", repository: "attacker/fork", batch: "42-1",
				}),
			},
			result: "success",
		},
		{
			name:    "empty plan plus undeclared",
			plan:    `[]`,
			reports: []string{exactCommandReport("beta", "2", "42-1")},
			result:  "skipped",
		},
		{
			name: "valid plus stale same identity",
			plan: `[{"name":"alpha","version":"1","type":"default"}]`,
			reports: []string{
				exactCommandReport("alpha", "1", "42-1"),
				exactCommandReport("alpha", "1", "41-1"),
			},
			result: "success",
		},
		{
			name: "valid plus stale undeclared",
			plan: `[{"name":"alpha","version":"1","type":"default"}]`,
			reports: []string{
				exactCommandReport("alpha", "1", "42-1"),
				exactCommandReport("beta", "2", "41-1"),
			},
			result: "success",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given: a hostile report set presented to the public workflowops command.
			tmp := t.TempDir()
			expected := filepath.Join(tmp, "expected.json")
			results := filepath.Join(tmp, "results")
			writeCommandFixture(t, expected, test.plan)
			for index, report := range test.reports {
				writeCommandFixture(t, filepath.Join(results, strconv.Itoa(index), "report.json"), report)
			}
			root := &cli.Command{Commands: []*cli.Command{New()}}

			// When: aggregation runs through CLI parsing and command wiring.
			err := root.Run(t.Context(), []string{
				"verity", "workflowops", "aggregate-integer-results", "--source-sha", commandSourceSHA,
				expected, results, test.result, "verity-org/verity", "42", "42-1",
			})

			// Then: no hostile extra can coexist with a successful outcome.
			require.Error(t, err)
		})
	}
}

const commandSourceSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type commandReportFixture struct {
	image      string
	version    string
	runID      string
	attempt    int
	source     string
	repository string
	batch      string
}

func exactCommandReport(image, version, batch string) string {
	return renderCommandReport(&commandReportFixture{
		image: image, version: version, runID: "42", attempt: 1,
		source: commandSourceSHA, repository: "verity-org/verity", batch: batch,
	})
}

func renderCommandReport(fixture *commandReportFixture) string {
	return `{"image":"` + fixture.image + `","version":"` + fixture.version + `","type":"default","status":"success",` +
		`"failure_stage":"","run_id":"` + fixture.runID + `","run_attempt":` + strconv.Itoa(fixture.attempt) +
		`,"source_sha":"` + fixture.source + `","repository":"` + fixture.repository + `","batch_id":"` + fixture.batch + `","shard":1}`
}

func writeCommandFixture(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
