package chartgen

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errSentinelTarRead = errors.New("sentinel tar read failure")

func Test_extractTarball_ignores_empty_and_special_entries_and_defaults_file_mode(t *testing.T) {
	// Given
	tgz := writeRawTarball(t, []*tar.Header{
		{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "chart/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "chart/pipe", Typeflag: tar.TypeFifo, Mode: 0o600},
		{Name: "chart/values.yaml", Typeflag: tar.TypeReg, Mode: 0},
	}, []string{"", "", "", "image: sentinel\n"})
	dest := t.TempDir()

	// When
	chartName, err := extractTarball(tgz, dest)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "chart", chartName)
	assert.NoFileExists(t, filepath.Join(dest, "chart", "pipe"))
	info, statErr := os.Stat(filepath.Join(dest, "chart", "values.yaml"))
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func Test_visitTarballEntries_reports_malformed_tar_stream(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "malformed.tgz")
	file, err := os.Create(path)
	require.NoError(t, err)
	gz := gzip.NewWriter(file)
	_, err = gz.Write([]byte("not a tar stream"))
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	require.NoError(t, file.Close())

	// When
	_, err = visitTarballEntries(path, func(*tar.Header, string, io.Reader) error { return nil })

	// Then
	require.ErrorContains(t, err, "read tarball")
}

func Test_visitTarballEntries_propagates_visitor_error(t *testing.T) {
	// Given
	tgz := writeTestChartTarball(t, "chart", map[string]string{"values.yaml": "image: sentinel\n"})

	// When
	_, err := visitTarballEntries(tgz, func(*tar.Header, string, io.Reader) error {
		return errSentinelTarRead
	})

	// Then
	require.ErrorIs(t, err, errSentinelTarRead)
}

func Test_readChartArchive_returns_values_when_chart_metadata_is_invalid(t *testing.T) {
	// Given
	tgz := writeTestChartTarball(t, "chart", map[string]string{
		"Chart.yaml":         "version: [\n",
		"nested/ignored.txt": "sentinel\n",
		"values.yaml":        "image: sentinel\n",
	})

	// When
	values, chartName, version, err := readChartArchive(tgz)

	// Then
	require.NoError(t, err)
	assert.Equal(t, "chart", chartName)
	assert.Empty(t, version)
	assert.Equal(t, "image: sentinel\n", string(values))
}

func Test_extractTarball_entry_helpers_reject_paths_outside_destination(t *testing.T) {
	// Given
	dest := t.TempDir()
	outside := filepath.Join(filepath.Dir(dest), "outside")

	// When
	dirErr := extractTarballDirEntry(dest, outside, "../outside")
	fileErr := extractTarballRegularEntry("sentinel.tgz", dest, outside, "../outside", &tar.Header{Mode: 0o644}, nil)

	// Then
	require.ErrorIs(t, dirErr, ErrUnsafeTarballEntry)
	require.ErrorIs(t, fileErr, ErrUnsafeTarballEntry)
}

func Test_extractTarballRegularEntry_reports_parent_open_and_copy_failures(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, dest string) (string, io.Reader)
		needle string
	}{
		{
			name: "parent path is a file",
			setup: func(t *testing.T, dest string) (string, io.Reader) {
				blocker := filepath.Join(dest, "blocker")
				require.NoError(t, os.WriteFile(blocker, []byte("sentinel"), 0o644))
				return filepath.Join(blocker, "values.yaml"), nil
			},
			needle: "create parent directory",
		},
		{
			name: "target path is a directory",
			setup: func(t *testing.T, dest string) (string, io.Reader) {
				target := filepath.Join(dest, "values.yaml")
				require.NoError(t, os.Mkdir(target, 0o755))
				return target, nil
			},
			needle: "open extracted file",
		},
		{
			name: "source reader fails",
			setup: func(_ *testing.T, dest string) (string, io.Reader) {
				return filepath.Join(dest, "values.yaml"), sentinelErrorReader{}
			},
			needle: "copy file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			dest := t.TempDir()
			target, reader := tt.setup(t, dest)

			// When
			err := extractTarballRegularEntry("sentinel.tgz", dest, target, "chart/values.yaml", &tar.Header{Mode: 0o644}, reader)

			// Then
			require.ErrorContains(t, err, tt.needle)
		})
	}
}

type sentinelErrorReader struct{}

func (sentinelErrorReader) Read([]byte) (int, error) {
	return 0, errSentinelTarRead
}
