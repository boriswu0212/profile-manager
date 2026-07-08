package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"0.1.0", "0.2.0", true},
		{"0.2.0", "0.1.0", false},
		{"0.1.0", "0.1.0", false},
		{"dev", "0.1.0", true},
		{"v0.1.0", "v0.2.0", true},
		{"0.1.0", "0.1.1", true},
		{"1.0.0", "0.9.9", false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.current, tt.latest), func(t *testing.T) {
			got := IsNewer(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		in   string
		want [3]int
	}{
		{"0.1.0", [3]int{0, 1, 0}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"dev", [3]int{0, 0, 0}},
		{"", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := parseVersion(tt.in)
			if got != tt.want {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello world")
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	t.Run("match", func(t *testing.T) {
		checksums := []byte(fmt.Sprintf("%s  pm_0.1.0_linux_amd64.tar.gz\n", hash))
		if err := verifyChecksum(data, checksums, "pm_0.1.0_linux_amd64.tar.gz"); err != nil {
			t.Errorf("verifyChecksum() = %v, want nil", err)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		wrongHash := strings.Repeat("0", 64)
		checksums := []byte(fmt.Sprintf("%s  pm_0.1.0_linux_amd64.tar.gz\n", wrongHash))
		err := verifyChecksum(data, checksums, "pm_0.1.0_linux_amd64.tar.gz")
		if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("verifyChecksum() = %v, want error containing %q", err, "checksum mismatch")
		}
	})

	t.Run("missing", func(t *testing.T) {
		checksums := []byte(fmt.Sprintf("%s  pm_0.1.0_darwin_arm64.tar.gz\n", hash))
		err := verifyChecksum(data, checksums, "pm_0.1.0_linux_amd64.tar.gz")
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("verifyChecksum() = %v, want error containing %q", err, "not found")
		}
	})
}

func buildTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	hdr := &tar.Header{
		Name: name,
		Mode: 0o755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	return buf.Bytes()
}

func TestExtractBinary(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		content := []byte("fake pm binary content")
		archive := buildTarGz(t, "pm", content)

		got, err := extractBinary(archive)
		if err != nil {
			t.Fatalf("extractBinary() error = %v", err)
		}
		if !bytes.Equal(got, content) {
			t.Errorf("extractBinary() = %q, want %q", got, content)
		}
	})

	t.Run("missing", func(t *testing.T) {
		archive := buildTarGz(t, "other", []byte("not the binary"))

		_, err := extractBinary(archive)
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("extractBinary() error = %v, want error containing %q", err, "not found")
		}
	})
}
