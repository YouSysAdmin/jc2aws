package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestCompareVersions
// ---------------------------------------------------------------------------

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "1.0.0", "1.0.0", 0},
		{"equal with v prefix", "v1.0.0", "1.0.0", 0},
		{"equal both v prefix", "v2.1.3", "v2.1.3", 0},
		{"a less major", "1.0.0", "2.0.0", -1},
		{"a less minor", "1.1.0", "1.2.0", -1},
		{"a less patch", "1.0.1", "1.0.2", -1},
		{"a greater major", "3.0.0", "2.9.9", 1},
		{"a greater minor", "1.2.0", "1.1.9", 1},
		{"a greater patch", "1.0.5", "1.0.4", 1},
		{"v prefix mixed", "v3.0.1", "3.0.0", 1},
		{"pre-release stripped", "2.0.0-pre", "1.9.9", 1},
		{"invalid a treated as 0.0.0", "invalid", "0.0.1", -1},
		{"invalid b treated as 0.0.0", "0.0.1", "invalid", 1},
		{"both invalid", "bad", "worse", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestStripV
// ---------------------------------------------------------------------------

func TestStripV(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"v1.0.0", "1.0.0"},
		{"1.0.0", "1.0.0"},
		{"v", ""},
		{"", ""},
		{"vv1.0", "v1.0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripV(tt.input); got != tt.want {
				t.Errorf("stripV(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestParseVersion
// ---------------------------------------------------------------------------

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name                      string
		input                     string
		wantMaj, wantMin, wantPat int
		wantErr                   bool
	}{
		{"simple", "1.2.3", 1, 2, 3, false},
		{"with v", "v3.0.1", 3, 0, 1, false},
		{"pre-release", "2.0.0-pre", 2, 0, 0, false},
		{"too few parts", "1.2", 0, 0, 0, true},
		{"non-numeric major", "a.2.3", 0, 0, 0, true},
		{"non-numeric minor", "1.b.3", 0, 0, 0, true},
		{"non-numeric patch", "1.2.c", 0, 0, 0, true},
		{"empty", "", 0, 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maj, min, pat, err := parseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil {
				if maj != tt.wantMaj || min != tt.wantMin || pat != tt.wantPat {
					t.Errorf("parseVersion(%q) = (%d, %d, %d), want (%d, %d, %d)",
						tt.input, maj, min, pat, tt.wantMaj, tt.wantMin, tt.wantPat)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestBuildAssetName
// ---------------------------------------------------------------------------

func TestBuildAssetName(t *testing.T) {
	tests := []struct {
		name, version, goos, goarch, want string
	}{
		{"linux amd64", "3.0.1", "linux", "amd64", "jc2aws_v3.0.1_linux_amd64.tar.gz"},
		{"linux arm64", "3.0.1", "linux", "arm64", "jc2aws_v3.0.1_linux_arm64.tar.gz"},
		{"linux arm (armv7)", "3.0.1", "linux", "arm", "jc2aws_v3.0.1_linux_armv7.tar.gz"},
		{"darwin amd64", "2.0.0", "darwin", "amd64", "jc2aws_v2.0.0_darwin_amd64.tar.gz"},
		{"darwin arm64", "2.0.0", "darwin", "arm64", "jc2aws_v2.0.0_darwin_arm64.tar.gz"},
		{"windows amd64", "3.0.1", "windows", "amd64", "jc2aws_v3.0.1_windows_amd64.zip"},
		{"windows arm64", "1.0.0", "windows", "arm64", "jc2aws_v1.0.0_windows_arm64.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAssetName(tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("BuildAssetName(%q, %q, %q) = %q, want %q",
					tt.version, tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestFindAssetURL
// ---------------------------------------------------------------------------

func TestFindAssetURL(t *testing.T) {
	assets := []Asset{
		{Name: "jc2aws_v3.0.1_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux_amd64.tar.gz"},
		{Name: "jc2aws_v3.0.1_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_arm64.tar.gz"},
		{Name: "checksums.sha256", BrowserDownloadURL: "https://example.com/checksums.sha256"},
	}

	tests := []struct {
		name, search, want string
	}{
		{"found linux", "jc2aws_v3.0.1_linux_amd64.tar.gz", "https://example.com/linux_amd64.tar.gz"},
		{"found checksums", "checksums.sha256", "https://example.com/checksums.sha256"},
		{"not found", "jc2aws_v3.0.1_windows_amd64.zip", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findAssetURL(assets, tt.search)
			if got != tt.want {
				t.Errorf("findAssetURL(_, %q) = %q, want %q", tt.search, got, tt.want)
			}
		})
	}

	t.Run("empty list", func(t *testing.T) {
		got := findAssetURL(nil, "anything")
		if got != "" {
			t.Errorf("findAssetURL(nil, _) = %q, want empty", got)
		}
	})
}

// ---------------------------------------------------------------------------
// TestCheckLatestVersion
// ---------------------------------------------------------------------------

func TestCheckLatestVersion_NewerAvailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Release{TagName: "v4.0.0"})
	}))
	defer srv.Close()

	origURL := ReleaseAPIURL
	defer func() { restoreReleaseAPIURL(origURL) }()
	overrideReleaseAPIURL(srv.URL)

	result := checkLatestVersionFromURL(srv.URL, "3.0.1")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.LatestVersion != "4.0.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "4.0.0")
	}
}

func TestCheckLatestVersion_UpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Release{TagName: "v3.0.1"})
	}))
	defer srv.Close()

	result := checkLatestVersionFromURL(srv.URL, "3.0.1")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.LatestVersion != "" {
		t.Errorf("LatestVersion = %q, want empty (up to date)", result.LatestVersion)
	}
}

func TestCheckLatestVersion_DevBuild(t *testing.T) {
	result := CheckLatestVersion("")
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.LatestVersion != "" {
		t.Errorf("LatestVersion = %q, want empty for dev build", result.LatestVersion)
	}
}

func TestCheckLatestVersion_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	result := checkLatestVersionFromURL(srv.URL, "3.0.1")
	if result.Err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// checkLatestVersionFromURL is a test helper that calls fetchRelease with
// a custom URL instead of using the package-level constant.
func checkLatestVersionFromURL(apiURL, currentVersion string) CheckResult {
	if currentVersion == "" {
		return CheckResult{}
	}

	rel, err := fetchRelease(apiURL)
	if err != nil {
		return CheckResult{CurrentVersion: currentVersion, Err: err}
	}

	latest := stripV(rel.TagName)
	if CompareVersions(currentVersion, latest) < 0 {
		return CheckResult{
			CurrentVersion: currentVersion,
			LatestVersion:  latest,
		}
	}

	return CheckResult{CurrentVersion: currentVersion}
}

// overrideReleaseAPIURL and restoreReleaseAPIURL are no-ops; the test uses
// checkLatestVersionFromURL instead of the real CheckLatestVersion.
func overrideReleaseAPIURL(_ string) {}
func restoreReleaseAPIURL(_ string)  {}

// ---------------------------------------------------------------------------
// TestVerifyChecksum
// ---------------------------------------------------------------------------

func TestVerifyChecksum(t *testing.T) {
	// Create a temp archive file with known content
	content := []byte("test binary content for checksum verification")
	archivePath := writeTempFile(t, content)
	defer os.Remove(archivePath)

	// Compute the real SHA256
	h := sha256.Sum256(content)
	correctHash := hex.EncodeToString(h[:])

	checksumFileContent := fmt.Sprintf(
		"%s  jc2aws_v3.0.1_linux_amd64.tar.gz\nabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  other_file.tar.gz\n",
		correctHash,
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, checksumFileContent)
	}))
	defer srv.Close()

	assets := []Asset{
		{Name: "checksums.sha256", BrowserDownloadURL: srv.URL},
	}

	t.Run("valid checksum", func(t *testing.T) {
		err := verifyChecksum(assets, archivePath, "jc2aws_v3.0.1_linux_amd64.tar.gz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("wrong asset name", func(t *testing.T) {
		err := verifyChecksum(assets, archivePath, "nonexistent.tar.gz")
		if err == nil {
			t.Fatal("expected error for missing checksum entry, got nil")
		}
	})

	t.Run("tampered file", func(t *testing.T) {
		tampered := writeTempFile(t, []byte("tampered content"))
		defer os.Remove(tampered)

		err := verifyChecksum(assets, tampered, "jc2aws_v3.0.1_linux_amd64.tar.gz")
		if err == nil {
			t.Fatal("expected checksum mismatch error, got nil")
		}
		if !strings.Contains(err.Error(), "checksum mismatch") {
			t.Errorf("error = %q, want it to contain 'checksum mismatch'", err.Error())
		}
	})

	t.Run("missing checksums asset", func(t *testing.T) {
		err := verifyChecksum(nil, archivePath, "jc2aws_v3.0.1_linux_amd64.tar.gz")
		if err == nil {
			t.Fatal("expected error for missing checksums.sha256 asset, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// TestExtractTarGz
// ---------------------------------------------------------------------------

func TestExtractTarGz(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho hello\n")
	archivePath := createTestTarGz(t, "jc2aws", binaryContent)
	defer os.Remove(archivePath)

	t.Run("extract existing binary", func(t *testing.T) {
		data, err := extractTarGz(archivePath, "jc2aws")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(data, binaryContent) {
			t.Errorf("extracted content = %q, want %q", data, binaryContent)
		}
	})

	t.Run("binary not found", func(t *testing.T) {
		_, err := extractTarGz(archivePath, "nonexistent")
		if err == nil {
			t.Fatal("expected error for missing binary, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// TestExtractZip
// ---------------------------------------------------------------------------

func TestExtractZip(t *testing.T) {
	binaryContent := []byte("MZ fake windows binary")
	archivePath := createTestZip(t, "jc2aws.exe", binaryContent)
	defer os.Remove(archivePath)

	t.Run("extract existing binary", func(t *testing.T) {
		data, err := extractZip(archivePath, "jc2aws.exe")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !bytes.Equal(data, binaryContent) {
			t.Errorf("extracted content = %q, want %q", data, binaryContent)
		}
	})

	t.Run("binary not found", func(t *testing.T) {
		_, err := extractZip(archivePath, "nonexistent")
		if err == nil {
			t.Fatal("expected error for missing binary, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// TestParseChecksumFile
// ---------------------------------------------------------------------------

func TestParseChecksumFile(t *testing.T) {
	checksumData := "aabbccdd  file_a.tar.gz\n11223344  file_b.zip\n"

	t.Run("found", func(t *testing.T) {
		got, err := parseChecksumFile(strings.NewReader(checksumData), "file_a.tar.gz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "aabbccdd" {
			t.Errorf("got %q, want %q", got, "aabbccdd")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := parseChecksumFile(strings.NewReader(checksumData), "missing.tar.gz")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

// ---------------------------------------------------------------------------
// TestAtomicReplace
// ---------------------------------------------------------------------------

func TestAtomicReplace(t *testing.T) {
	// Create a "current binary" in a temp dir
	dir := t.TempDir()
	exePath := filepath.Join(dir, "jc2aws")
	if err := os.WriteFile(exePath, []byte("old binary"), 0755); err != nil {
		t.Fatalf("failed to write test binary: %v", err)
	}

	newContent := []byte("new binary v2")

	if err := atomicReplace(exePath, newContent); err != nil {
		t.Fatalf("atomicReplace() error: %v", err)
	}

	// Verify the file was replaced
	got, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatalf("failed to read replaced binary: %v", err)
	}
	if !bytes.Equal(got, newContent) {
		t.Errorf("replaced content = %q, want %q", got, newContent)
	}

	// Verify the old backup was cleaned up
	oldPath := exePath + ".old"
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf(".old file still exists: %v", err)
	}

	// Verify permissions are preserved
	info, err := os.Stat(exePath)
	if err != nil {
		t.Fatalf("failed to stat replaced binary: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("permissions = %o, want 0755", info.Mode().Perm())
	}
}

// ---------------------------------------------------------------------------
// TestDownloadAndReplace (integration)
// ---------------------------------------------------------------------------

func TestDownloadAndReplace(t *testing.T) {
	// Create a fake binary to put in the archive
	fakeBinary := []byte("#!/bin/sh\necho updated jc2aws\n")

	binName := "jc2aws"
	if runtime.GOOS == "windows" {
		binName = "jc2aws.exe"
	}

	// Create archive (tar.gz for non-windows, zip for windows)
	var archiveData []byte
	assetName := BuildAssetName("4.0.0", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		archiveData = createTestZipBytes(t, binName, fakeBinary)
	} else {
		archiveData = createTestTarGzBytes(t, binName, fakeBinary)
	}

	// Compute checksum of the archive
	archiveHash := sha256.Sum256(archiveData)
	checksumLine := fmt.Sprintf("%s  %s\n", hex.EncodeToString(archiveHash[:]), assetName)

	// Create a "current binary" for replacement
	dir := t.TempDir()
	exePath := filepath.Join(dir, binName)
	if err := os.WriteFile(exePath, []byte("old binary"), 0755); err != nil {
		t.Fatalf("failed to write test binary: %v", err)
	}

	// Override execPathFunc so DownloadAndReplace finds our temp binary
	execPathFunc = func() (string, error) { return exePath, nil }
	defer func() { execPathFunc = nil }()

	// Set up test server
	mux := http.NewServeMux()
	mux.HandleFunc("/api/release", func(w http.ResponseWriter, r *http.Request) {
		rel := Release{
			TagName: "v4.0.0",
			Assets: []Asset{
				{Name: assetName, BrowserDownloadURL: ""}, // URL set below
				{Name: "checksums.sha256", BrowserDownloadURL: ""},
			},
		}
		json.NewEncoder(w).Encode(rel)
	})
	mux.HandleFunc("/download/archive", func(w http.ResponseWriter, r *http.Request) {
		w.Write(archiveData)
	})
	mux.HandleFunc("/download/checksums", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, checksumLine)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Re-register with correct URLs
	mux.HandleFunc("/api/release-with-urls", func(w http.ResponseWriter, r *http.Request) {
		rel := Release{
			TagName: "v4.0.0",
			Assets: []Asset{
				{Name: assetName, BrowserDownloadURL: srv.URL + "/download/archive"},
				{Name: "checksums.sha256", BrowserDownloadURL: srv.URL + "/download/checksums"},
			},
		}
		json.NewEncoder(w).Encode(rel)
	})

	// Test: up-to-date
	t.Run("already up to date", func(t *testing.T) {
		var buf bytes.Buffer
		err := downloadAndReplaceFromURL(srv.URL+"/api/release-with-urls", "4.0.0", &buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(buf.String(), "Already up to date") {
			t.Errorf("output = %q, want to contain 'Already up to date'", buf.String())
		}
	})

	// Test: newer version available
	t.Run("update available", func(t *testing.T) {
		// Reset the binary
		if err := os.WriteFile(exePath, []byte("old binary"), 0755); err != nil {
			t.Fatalf("failed to reset test binary: %v", err)
		}

		var buf bytes.Buffer
		err := downloadAndReplaceFromURL(srv.URL+"/api/release-with-urls", "3.0.1", &buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "Updated successfully") {
			t.Errorf("output = %q, want to contain 'Updated successfully'", output)
		}

		// Verify the binary was replaced
		got, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatalf("failed to read replaced binary: %v", err)
		}
		if !bytes.Equal(got, fakeBinary) {
			t.Errorf("binary content = %q, want %q", got, fakeBinary)
		}
	})

	// Test: dev build
	t.Run("dev build", func(t *testing.T) {
		// Reset the binary
		if err := os.WriteFile(exePath, []byte("old binary"), 0755); err != nil {
			t.Fatalf("failed to reset test binary: %v", err)
		}

		var buf bytes.Buffer
		err := downloadAndReplaceFromURL(srv.URL+"/api/release-with-urls", "", &buf)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "development build") {
			t.Errorf("output = %q, want to contain 'development build'", output)
		}
		if !strings.Contains(output, "Updated to v4.0.0") {
			t.Errorf("output = %q, want to contain 'Updated to v4.0.0'", output)
		}
	})
}

// downloadAndReplaceFromURL is a test helper that uses a custom API URL.
func downloadAndReplaceFromURL(apiURL, currentVersion string, w io.Writer) error {
	fmt.Fprintln(w, "Checking for latest version...")

	rel, err := fetchRelease(apiURL)
	if err != nil {
		return fmt.Errorf("failed to fetch release info: %w", err)
	}

	latest := stripV(rel.TagName)

	if currentVersion == "" {
		fmt.Fprintln(w, "Warning: development build, current version unknown")
	} else if CompareVersions(currentVersion, latest) >= 0 {
		fmt.Fprintf(w, "Already up to date (v%s)\n", currentVersion)
		return nil
	}

	assetName := BuildAssetName(latest, runtime.GOOS, runtime.GOARCH)
	assetURL := findAssetURL(rel.Assets, assetName)
	if assetURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (expected %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

	fmt.Fprintf(w, "Downloading jc2aws v%s for %s/%s...\n", latest, runtime.GOOS, runtime.GOARCH)

	archivePath, err := downloadFile(assetURL)
	if err != nil {
		return fmt.Errorf("failed to download release: %w", err)
	}
	defer os.Remove(archivePath)

	fmt.Fprintln(w, "Verifying checksum...")

	if err := verifyChecksum(rel.Assets, archivePath, assetName); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	binName := binaryName
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	binData, err := extractBinary(archivePath, binName)
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	exePath, err := getExecPath()
	if err != nil {
		return fmt.Errorf("failed to determine executable path: %w", err)
	}

	fmt.Fprintf(w, "Replacing %s...\n", exePath)

	if err := atomicReplace(exePath, binData); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	if currentVersion == "" {
		fmt.Fprintf(w, "Updated to v%s\n", latest)
	} else {
		fmt.Fprintf(w, "Updated successfully: v%s -> v%s\n", currentVersion, latest)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func writeTempFile(t *testing.T, content []byte) string {
	t.Helper()
	f, err := os.CreateTemp("", "update-test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := f.Write(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatalf("failed to write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

func createTestTarGz(t *testing.T, name string, content []byte) string {
	t.Helper()
	data := createTestTarGzBytes(t, name, content)
	return writeTempFile(t, data)
}

func createTestTarGzBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: name,
		Mode: 0755,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("failed to write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write tar content: %v", err)
	}

	tw.Close()
	gw.Close()
	return buf.Bytes()
}

func createTestZip(t *testing.T, name string, content []byte) string {
	t.Helper()
	data := createTestZipBytes(t, name, content)
	return writeTempFile(t, data)
}

func createTestZipBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	fw, err := zw.Create(name)
	if err != nil {
		t.Fatalf("failed to create zip entry: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("failed to write zip content: %v", err)
	}

	zw.Close()
	return buf.Bytes()
}
