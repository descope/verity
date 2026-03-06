package apkindex

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeAPKINDEXTarGz builds a minimal tar.gz containing an APKINDEX file.
func makeAPKINDEXTarGz(content string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	data := []byte(content)
	_ = tw.WriteHeader(&tar.Header{ //nolint:errcheck // test helper, errors not meaningful
		Name: "APKINDEX",
		Mode: 0o644,
		Size: int64(len(data)),
	})
	_, _ = tw.Write(data) //nolint:errcheck // test helper, errors not meaningful
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestFetch_Success(t *testing.T) {
	apkindex := "P:nodejs-22\nV:22.0.0\n\nP:nodejs-24\nV:24.0.0\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		_, _ = w.Write(makeAPKINDEXTarGz(apkindex)) //nolint:errcheck // test helper, errors not meaningful
	}))
	defer srv.Close()

	pkgs, err := Fetch(srv.URL, "", 0)
	require.NoError(t, err)
	require.Len(t, pkgs, 2)
	assert.Equal(t, "nodejs-22", pkgs[0].Name)
	assert.Equal(t, "nodejs-24", pkgs[1].Name)
}

func TestFetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Fetch(srv.URL, "", 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestFetch_CacheHit(t *testing.T) {
	cacheDir := t.TempDir()

	apkindex := "P:curl\nV:8.0.0\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(makeAPKINDEXTarGz(apkindex)) //nolint:errcheck // test helper, errors not meaningful
	}))
	defer srv.Close()

	// First call populates the cache.
	pkgs1, err := Fetch(srv.URL, cacheDir, time.Hour)
	require.NoError(t, err)
	require.Len(t, pkgs1, 1)

	// Second call should hit cache (server is still up but we verify same result).
	pkgs2, err := Fetch(srv.URL, cacheDir, time.Hour)
	require.NoError(t, err)
	require.Len(t, pkgs2, 1)
	assert.Equal(t, pkgs1[0].Name, pkgs2[0].Name)
}

func TestFetch_ExpiredCache(t *testing.T) {
	cacheDir := t.TempDir()

	apkindex := "P:curl\nV:8.0.0\n\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(makeAPKINDEXTarGz(apkindex)) //nolint:errcheck // test helper, errors not meaningful
	}))
	defer srv.Close()

	// Populate cache.
	_, err := Fetch(srv.URL, cacheDir, time.Hour)
	require.NoError(t, err)

	// With maxAge=0, cache is bypassed.
	pkgs, err := Fetch(srv.URL, cacheDir, 0)
	require.NoError(t, err)
	assert.Len(t, pkgs, 1)
}

func TestFetch_BadURL(t *testing.T) {
	_, err := Fetch("http://127.0.0.1:0/nonexistent", "", 0)
	require.Error(t, err)
}

func TestParseTarGz_MissingAPKINDEX(t *testing.T) {
	// Build a tar.gz that contains a different file, not APKINDEX.
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "other.txt", Mode: 0o644, Size: 4}) //nolint:errcheck // test helper, errors not meaningful
	_, _ = tw.Write([]byte("data"))                                          //nolint:errcheck // test helper, errors not meaningful
	tw.Close()
	gz.Close()

	_, err := parseTarGz(&buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "APKINDEX file not found")
}

func TestParseTarGz_InvalidGzip(t *testing.T) {
	buf := bytes.NewBufferString("not gzip data")
	_, err := parseTarGz(buf)
	require.Error(t, err)
}
