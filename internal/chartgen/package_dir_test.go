package chartgen

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/verity-org/verity/internal/config"
)

var errTestCloseFailed = errors.New("close failed")

func TestMovePackagedChartWritesIntoPackageDir(t *testing.T) {
	srcDir := t.TempDir()
	packageDir := t.TempDir()
	src := filepath.Join(srcDir, "cert-manager-v1.21.0.tgz")
	if err := os.WriteFile(src, []byte("chart"), 0o644); err != nil {
		t.Fatalf("write source package: %v", err)
	}

	dest, err := movePackagedChart(src, packageDir)
	if err != nil {
		t.Fatalf("movePackagedChart() error = %v", err)
	}

	want := filepath.Join(packageDir, "cert-manager-v1.21.0.tgz")
	if dest != want {
		t.Fatalf("movePackagedChart() = %q, want %q", dest, want)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source package still exists or stat failed differently: %v", err)
	}
	if got, err := os.ReadFile(want); err != nil || string(got) != "chart" {
		t.Fatalf("dest package = %q, %v; want chart", got, err)
	}
}

func TestMovePackagedChartCopiesAcrossFilesystems(t *testing.T) {
	if _, err := os.Stat("/dev/shm"); err != nil {
		t.Skip("/dev/shm unavailable")
	}
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "cert-manager-v1.21.0.tgz")
	content := []byte("chart")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write source package: %v", err)
	}
	packageDir, err := os.MkdirTemp("/dev/shm", "verity-chartgen-*")
	if err != nil {
		t.Skipf("create /dev/shm package dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(packageDir) })

	got, err := movePackagedChart(src, packageDir)
	if err != nil {
		t.Fatalf("movePackagedChart() error = %v", err)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source stat error = %v, want not exist", err)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatalf("read destination package: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("destination content = %q, want %q", data, content)
	}
}

func TestCopyFileReportsMissingSource(t *testing.T) {
	err := copyFile(filepath.Join(t.TempDir(), "missing.tgz"), filepath.Join(t.TempDir(), "dest.tgz"))
	if err == nil {
		t.Fatal("copyFile() error = nil, want missing source error")
	}
}

func TestCopyFileOverwritesDestination(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.tgz")
	dest := filepath.Join(dir, "dest.tgz")
	content := []byte("new")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(dest, []byte("old-content"), 0o644); err != nil {
		t.Fatalf("write destination: %v", err)
	}

	if err := copyFile(src, dest); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("destination content = %q, want %q", data, content)
	}
}

func TestCopyToSyncWriteCloserReportsCloseError(t *testing.T) {
	out := &failingCloseWriter{closeErr: errTestCloseFailed}

	err := copyToSyncWriteCloser(out, strings.NewReader("chart"))
	if !errors.Is(err, errTestCloseFailed) {
		t.Fatalf("copyToSyncWriteCloser() error = %v, want close error", err)
	}
	if got := out.String(); got != "chart" {
		t.Fatalf("written content = %q, want chart", got)
	}
}

func TestMovePackagedChartReportsCreateDirError(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "cert-manager-v1.21.0.tgz")
	if err := os.WriteFile(src, []byte("chart"), 0o644); err != nil {
		t.Fatalf("write source package: %v", err)
	}
	parentFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parentFile, []byte("file"), 0o644); err != nil {
		t.Fatalf("write parent file: %v", err)
	}

	_, err := movePackagedChart(src, filepath.Join(parentFile, "child"))
	if err == nil {
		t.Fatal("movePackagedChart() error = nil, want create-dir error")
	}
}

func TestSelectChartsKeepsOnlyRequestedChart(t *testing.T) {
	charts := []config.ChartSpec{
		{Name: "prometheus", Version: "29.14.0"},
		{Name: "cert-manager", Version: "v1.21.0"},
	}

	got, err := selectCharts(charts, "cert-manager")
	if err != nil {
		t.Fatalf("selectCharts() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "cert-manager" || got[0].Version != "v1.21.0" {
		t.Fatalf("selectCharts() = %#v, want only cert-manager v1.21.0", got)
	}
}

func TestSelectChartsEmptyNameKeepsAllCharts(t *testing.T) {
	charts := []config.ChartSpec{
		{Name: "prometheus", Version: "29.14.0"},
		{Name: "cert-manager", Version: "v1.21.0"},
	}

	got, err := selectCharts(charts, " ")
	if err != nil {
		t.Fatalf("selectCharts() error = %v", err)
	}
	if len(got) != len(charts) {
		t.Fatalf("selectCharts() length = %d, want %d", len(got), len(charts))
	}
}

func TestSelectChartsRejectsUnknownChart(t *testing.T) {
	_, err := selectCharts([]config.ChartSpec{{Name: "prometheus"}}, "cert-manager")
	if err == nil {
		t.Fatal("selectCharts() error = nil, want unknown chart error")
	}
	if !errors.Is(err, ErrChartNotFound) {
		t.Fatalf("selectCharts() error = %v, want ErrChartNotFound", err)
	}
}

type failingCloseWriter struct {
	bytes.Buffer
	closeErr error
}

func (w *failingCloseWriter) Sync() error {
	return nil
}

func (w *failingCloseWriter) Close() error {
	return w.closeErr
}
