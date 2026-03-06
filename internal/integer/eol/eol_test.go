package eol

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCycle_EOLDate(t *testing.T) {
	tests := []struct {
		name     string
		eol      any
		wantDate string
	}{
		{"string date", "2026-02-11", "2026-02-11"},
		{"false boolean", false, ""},
		{"nil", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Cycle{EOL: tt.eol}
			if got := c.EOLDate(); got != tt.wantDate {
				t.Errorf("EOLDate() = %q, want %q", got, tt.wantDate)
			}
		})
	}
}

func TestClient_FetchCycles(t *testing.T) {
	// Mock server returning Go cycles
	cycles := []Cycle{
		{Cycle: "1.26", EOL: false, Latest: "1.26.0"},
		{Cycle: "1.25", EOL: false, Latest: "1.25.7"},
		{Cycle: "1.24", EOL: "2026-02-11", Latest: "1.24.13"},
		{Cycle: "1.23", EOL: "2025-08-12", Latest: "1.23.12"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/go.json" {
			if err := json.NewEncoder(w).Encode(cycles); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		if r.URL.Path == "/unknown.json" {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := &Client{
		httpClient: http.DefaultClient,
		baseURL:    server.URL,
	}

	t.Run("existing product", func(t *testing.T) {
		got, err := client.FetchCycles("go")
		if err != nil {
			t.Fatalf("FetchCycles() error = %v", err)
		}
		if len(got) != 4 {
			t.Errorf("FetchCycles() returned %d cycles, want 4", len(got))
		}
		// Check that 1.24 has EOL date
		for _, c := range got {
			if c.Cycle == "1.24" && c.EOLDate() != "2026-02-11" {
				t.Errorf("1.24 EOLDate() = %q, want %q", c.EOLDate(), "2026-02-11")
			}
		}
	})

	t.Run("unknown product", func(t *testing.T) {
		got, err := client.FetchCycles("unknown")
		if err != nil {
			t.Fatalf("FetchCycles() error = %v", err)
		}
		if got != nil {
			t.Errorf("FetchCycles() = %v, want nil for unknown product", got)
		}
	})
}

func TestClient_FetchForImage(t *testing.T) {
	cycles := []Cycle{
		{Cycle: "1.26", EOL: false},
		{Cycle: "1.25", EOL: false},
		{Cycle: "1.24", EOL: "2026-02-11"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/go.json" {
			if err := json.NewEncoder(w).Encode(cycles); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &Client{
		httpClient: http.DefaultClient,
		baseURL:    server.URL,
	}

	t.Run("mapped image", func(t *testing.T) {
		data, err := client.FetchForImage("golang")
		if err != nil {
			t.Fatalf("FetchForImage() error = %v", err)
		}
		if data == nil {
			t.Fatal("FetchForImage() returned nil")
		}
		if got := data.LookupEOL("1.24"); got != "2026-02-11" {
			t.Errorf("LookupEOL(1.24) = %q, want %q", got, "2026-02-11")
		}
		if got := data.LookupEOL("1.26"); got != "" {
			t.Errorf("LookupEOL(1.26) = %q, want empty (not EOL)", got)
		}
	})

	t.Run("unmapped image", func(t *testing.T) {
		data, err := client.FetchForImage("custom-image")
		if err != nil {
			t.Fatalf("FetchForImage() error = %v", err)
		}
		if len(data) != 0 {
			t.Errorf("FetchForImage() = %v, want empty map for unmapped image", data)
		}
	})
}

func TestEOLData_LookupEOL(t *testing.T) {
	data := EOLData{
		"1.24": "2026-02-11",
		"1.23": "2025-08-12",
		"1.26": "",
	}

	tests := []struct {
		version string
		want    string
	}{
		{"1.24", "2026-02-11"},
		{"1.23", "2025-08-12"},
		{"1.26", ""},
		{"1.99", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := data.LookupEOL(tt.version); got != tt.want {
				t.Errorf("LookupEOL(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}

	t.Run("nil data", func(t *testing.T) {
		var nilData EOLData
		if got := nilData.LookupEOL("1.24"); got != "" {
			t.Errorf("LookupEOL on nil = %q, want empty", got)
		}
	})
}

func TestClient_FailFast(t *testing.T) {
	client := NewClientWithHTTP(http.DefaultClient, "http://localhost:1")

	_, err1 := client.FetchCycles("go")
	if err1 == nil {
		t.Fatal("expected error on first call to unreachable server")
	}

	_, err2 := client.FetchCycles("python")
	if err2 == nil {
		t.Fatal("expected error on second call")
	}
	if !errors.Is(err2, ErrAPIUnavailable) {
		t.Errorf("second call error = %v, want ErrAPIUnavailable", err2)
	}
}

func TestNewClientWithHTTP(t *testing.T) {
	customClient := &http.Client{}
	client := NewClientWithHTTP(customClient, "https://custom.api")

	if client.httpClient != customClient {
		t.Error("httpClient not set correctly")
	}
	if client.baseURL != "https://custom.api" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "https://custom.api")
	}
}
