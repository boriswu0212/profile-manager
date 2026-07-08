// Package updater implements self-update for the pm CLI from GitHub Releases.
package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const repo = "b0riswu/profile-manager"

// Release describes an available pm release.
type Release struct {
	Version     string
	ArchiveURL  string
	ChecksumURL string
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Latest queries the GitHub API for the latest pm release and resolves the
// archive and checksums asset URLs for the current OS/arch.
func Latest(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	version := strings.TrimPrefix(rel.TagName, "v")
	archiveName := fmt.Sprintf("pm_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)

	archiveURL := findAsset(rel.Assets, archiveName)
	if archiveURL == "" {
		return nil, fmt.Errorf("no release asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	checksumURL := findAsset(rel.Assets, "checksums.txt")

	return &Release{
		Version:     version,
		ArchiveURL:  archiveURL,
		ChecksumURL: checksumURL,
	}, nil
}

// IsNewer reports whether latest is a greater semantic version than current.
// "dev" is treated as 0.0.0 so any real release is considered newer.
func IsNewer(current, latest string) bool {
	c := parseVersion(current)
	l := parseVersion(latest)
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// Apply downloads rel's archive, verifies its checksum, extracts the pm
// binary, and atomically replaces targetPath with it.
func Apply(ctx context.Context, rel *Release, targetPath string) error {
	archive, err := download(ctx, rel.ArchiveURL)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	checksums, err := download(ctx, rel.ChecksumURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}

	archiveName := fmt.Sprintf("pm_%s_%s_%s.tar.gz", rel.Version, runtime.GOOS, runtime.GOARCH)
	if err := verifyChecksum(archive, checksums, archiveName); err != nil {
		return fmt.Errorf("verify checksum: %w", err)
	}

	binary, err := extractBinary(archive)
	if err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	dir := filepath.Dir(targetPath)
	tmp, err := os.CreateTemp(dir, "pm-update-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("set permissions: %w", err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("replace binary: permission denied — try running with sudo: %w", err)
		}
		return fmt.Errorf("replace binary: %w", err)
	}

	return nil
}

func download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return data, nil
}

func verifyChecksum(data []byte, checksums []byte, filename string) error {
	sum := sha256.Sum256(data)
	want := hex.EncodeToString(sum[:])

	for _, line := range strings.Split(string(checksums), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		hash, name := fields[0], fields[1]
		name = strings.TrimPrefix(name, "*")
		if name == filename {
			if hash != want {
				return fmt.Errorf("checksum mismatch for %s: got %s, want %s", filename, want, hash)
			}
			return nil
		}
	}

	return fmt.Errorf("%s not found in checksums", filename)
}

func extractBinary(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}

		name := strings.TrimPrefix(hdr.Name, "./")
		if name == "pm" {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("read entry: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("pm binary not found in archive")
}

func findAsset(assets []ghAsset, name string) string {
	for _, a := range assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func parseVersion(s string) [3]int {
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return [3]int{0, 0, 0}
	}

	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{0, 0, 0}
		}
		v[i] = n
	}
	return v
}
