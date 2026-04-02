package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
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
	"time"
)

const (
	// ReleaseAPIURL is the GitHub API endpoint for the latest release.
	ReleaseAPIURL = "https://api.github.com/repos/YouSysAdmin/jc2aws/releases/latest"

	// RepoURL is the GitHub repository URL.
	RepoURL = "https://github.com/YouSysAdmin/jc2aws"

	binaryName = "jc2aws"
)

// Release holds version info and asset URLs from a GitHub release.
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a single downloadable file in a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// CheckResult holds the result of a version check.
type CheckResult struct {
	CurrentVersion string
	LatestVersion  string // non-empty only when a newer version is available
	Err            error
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// CheckLatestVersion fetches the latest release from GitHub and compares it
// against currentVersion. If currentVersion is empty (dev build), no check
// is performed and an empty result is returned.
func CheckLatestVersion(currentVersion string) CheckResult {
	if currentVersion == "" {
		return CheckResult{}
	}

	rel, err := fetchRelease(ReleaseAPIURL)
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

// DownloadAndReplace downloads the latest release from GitHub and replaces
// the currently running binary. Progress messages are written to w.
func DownloadAndReplace(currentVersion string, w io.Writer) error {
	fmt.Fprintln(w, "Checking for latest version...")

	rel, err := fetchRelease(ReleaseAPIURL)
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

// CompareVersions compares two semantic version strings.
// Returns -1 if a < b, 0 if a == b, +1 if a > b.
// Leading "v" prefixes are stripped before comparison.
func CompareVersions(a, b string) int {
	aMaj, aMin, aPat, aErr := parseVersion(stripV(a))
	bMaj, bMin, bPat, bErr := parseVersion(stripV(b))

	// Unparseable versions are treated as "0.0.0"
	if aErr != nil {
		aMaj, aMin, aPat = 0, 0, 0
	}
	if bErr != nil {
		bMaj, bMin, bPat = 0, 0, 0
	}

	if aMaj != bMaj {
		return cmpInt(aMaj, bMaj)
	}
	if aMin != bMin {
		return cmpInt(aMin, bMin)
	}
	return cmpInt(aPat, bPat)
}

// BuildAssetName constructs the expected release asset filename for the
// given version, OS, and architecture, matching the GoReleaser naming
// convention.
func BuildAssetName(version, goos, goarch string) string {
	arch := goarch
	if goos == "linux" && goarch == "arm" {
		arch = "armv7"
	}

	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}

	return fmt.Sprintf("%s_v%s_%s_%s%s", binaryName, version, goos, arch, ext)
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

func fetchRelease(apiURL string) (Release, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("failed to parse release JSON: %w", err)
	}

	return rel, nil
}

func findAssetURL(assets []Asset, name string) string {
	for _, a := range assets {
		if a.Name == name {
			return a.BrowserDownloadURL
		}
	}
	return ""
}

func downloadFile(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp("", "jc2aws-update-*")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}

	return tmp.Name(), nil
}

func verifyChecksum(assets []Asset, archivePath, assetName string) error {
	checksumURL := findAssetURL(assets, "checksums.sha256")
	if checksumURL == "" {
		return fmt.Errorf("checksums.sha256 not found in release assets")
	}

	resp, err := httpClient.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksum download returned status %d", resp.StatusCode)
	}

	expected, err := parseChecksumFile(resp.Body, assetName)
	if err != nil {
		return err
	}

	actual, err := hashFile(archivePath)
	if err != nil {
		return err
	}

	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	return nil
}

func parseChecksumFile(r io.Reader, targetName string) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		// Format: "<hex>  <filename>"
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == targetName {
			return parts[0], nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("error reading checksums: %w", err)
	}
	return "", fmt.Errorf("checksum for %s not found", targetName)
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractBinary(archivePath, binaryName string) ([]byte, error) {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, binaryName)
	}
	return extractTarGz(archivePath, binaryName)
}

func extractTarGz(archivePath, binaryName string) ([]byte, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading tar: %w", err)
		}

		// Match the binary by base name (archives may have directory prefixes)
		if filepath.Base(hdr.Name) == binaryName && hdr.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, fmt.Errorf("error reading binary from archive: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("binary %s not found in archive", binaryName)
}

func extractZip(archivePath, binaryName string) ([]byte, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if filepath.Base(f.Name) == binaryName {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("error opening %s in zip: %w", f.Name, err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("error reading binary from zip: %w", err)
			}
			return data, nil
		}
	}

	return nil, fmt.Errorf("binary %s not found in zip archive", binaryName)
}

// executablePath returns the resolved path of the currently running binary.
func executablePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

func atomicReplace(exePath string, data []byte) error {
	// Determine permissions from the existing binary
	info, err := os.Stat(exePath)
	if err != nil {
		return err
	}
	mode := info.Mode().Perm()

	dir := filepath.Dir(exePath)
	newPath := exePath + ".new"
	oldPath := exePath + ".old"

	// Write the new binary to a temp location in the same directory
	if err := os.WriteFile(newPath, data, mode); err != nil {
		return fmt.Errorf("failed to write new binary to %s: %w", dir, err)
	}

	// Rename the current binary to .old, then the new one into place
	if err := os.Rename(exePath, oldPath); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("failed to move current binary: %w", err)
	}

	if err := os.Rename(newPath, exePath); err != nil {
		// Try to restore the old binary
		_ = os.Rename(oldPath, exePath)
		return fmt.Errorf("failed to move new binary into place: %w", err)
	}

	// Clean up the old binary
	os.Remove(oldPath)

	return nil
}

func stripV(version string) string {
	return strings.TrimPrefix(version, "v")
}

func parseVersion(s string) (major, minor, patch int, err error) {
	s = stripV(s)
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", s)
	}

	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version: %w", err)
	}

	// Handle pre-release suffixes like "1.0.0-pre" — take only the numeric part
	patchStr := parts[2]
	if idx := strings.IndexByte(patchStr, '-'); idx >= 0 {
		patchStr = patchStr[:idx]
	}
	patch, err = strconv.Atoi(patchStr)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version: %w", err)
	}

	return major, minor, patch, nil
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Testing helpers
// ---------------------------------------------------------------------------

// SetHTTPClient replaces the package-level HTTP client. Intended for tests.
func SetHTTPClient(c *http.Client) {
	httpClient = c
}

// SetExecPathFunc overrides the function used to determine the current
// executable path. Intended for tests. Pass nil to restore the default.
var execPathFunc func() (string, error)

func init() {
	execPathFunc = nil
}

// overridden in tests
func getExecPath() (string, error) {
	if execPathFunc != nil {
		return execPathFunc()
	}
	return executablePath()
}
