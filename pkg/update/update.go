package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

	// apiTimeout bounds the GitHub API and checksum requests.
	apiTimeout = 30 * time.Second
	// downloadTimeout bounds the release archive download, which can be
	// large and slow; it must not share the short API budget.
	downloadTimeout = 10 * time.Minute

	// maxBinarySize caps how much data is extracted from a release archive,
	// protecting against decompression bombs.
	maxBinarySize = 512 << 20 // 512 MiB
)

// userAgent identifies the updater to the GitHub API (unauthenticated
// requests without a User-Agent are rejected or rate-limited).
const userAgent = "jc2aws-updater (+" + RepoURL + ")"

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
func CheckLatestVersion(ctx context.Context, currentVersion string) CheckResult {
	if currentVersion == "" {
		return CheckResult{}
	}

	rel, err := fetchRelease(ctx, ReleaseAPIURL)
	if err != nil {
		return CheckResult{CurrentVersion: currentVersion, Err: err}
	}

	latest := stripV(rel.TagName)
	if _, _, _, err := parseVersion(latest); err != nil {
		return CheckResult{
			CurrentVersion: currentVersion,
			Err:            fmt.Errorf("cannot parse latest release tag %q: %w", rel.TagName, err),
		}
	}

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
func DownloadAndReplace(ctx context.Context, currentVersion string, w io.Writer) error {
	fmt.Fprintln(w, "Checking for latest version...")

	rel, err := fetchRelease(ctx, ReleaseAPIURL)
	if err != nil {
		return fmt.Errorf("failed to fetch release info: %w", err)
	}

	latest := stripV(rel.TagName)
	if _, _, _, err := parseVersion(latest); err != nil {
		return fmt.Errorf("cannot parse latest release tag %q: %w", rel.TagName, err)
	}

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

	archivePath, err := downloadFile(ctx, assetURL)
	if err != nil {
		return fmt.Errorf("failed to download release: %w", err)
	}
	defer os.Remove(archivePath)

	fmt.Fprintln(w, "Verifying checksum...")

	if err := verifyChecksum(ctx, rel.Assets, archivePath, assetName); err != nil {
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
		return cmp.Compare(aMaj, bMaj)
	}
	if aMin != bMin {
		return cmp.Compare(aMin, bMin)
	}
	return cmp.Compare(aPat, bPat)
}

// BuildAssetName constructs the expected release asset filename for the
// given version, OS, and architecture, matching the GoReleaser naming
// convention.
func BuildAssetName(version, goos, goarch string) string {
	arch := goarch
	// Mirror GoReleaser's name_template arch overrides.
	switch goarch {
	case "arm":
		arch = "armv7"
	case "386":
		arch = "i386"
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
	Transport: &http.Transport{
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

func fetchRelease(ctx context.Context, apiURL string) (Release, error) {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

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
	if i := slices.IndexFunc(assets, func(a Asset) bool { return a.Name == name }); i >= 0 {
		return assets[i].BrowserDownloadURL
	}
	return ""
}

func downloadFile(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
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

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("failed to write %s: %w", tmp.Name(), err)
	}

	return tmp.Name(), nil
}

func verifyChecksum(ctx context.Context, assets []Asset, archivePath, assetName string) error {
	checksumURL := findAssetURL(assets, "checksums.sha256")
	if checksumURL == "" {
		return fmt.Errorf("checksums.sha256 not found in release assets")
	}

	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checksumURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
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
		// Format: "<hex>  <filename>" — the name may carry a leading "*"
		// (binary-mode marker) or a directory prefix.
		parts := strings.Fields(scanner.Text())
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimPrefix(parts[len(parts)-1], "*")
		if filepath.Base(name) == targetName {
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

// readLimited reads all of r, failing if the content exceeds maxBinarySize.
func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBinarySize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBinarySize {
		return nil, fmt.Errorf("extracted file exceeds %d bytes", maxBinarySize)
	}
	return data, nil
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
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading tar: %w", err)
		}

		// Match the binary by base name (archives may have directory prefixes)
		if filepath.Base(hdr.Name) == binaryName && hdr.Typeflag == tar.TypeReg {
			data, err := readLimited(tr)
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
		// Only regular files count — a directory entry named like the binary
		// would otherwise be extracted as zero bytes.
		if filepath.Base(f.Name) != binaryName || !f.FileInfo().Mode().IsRegular() {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("error opening %s in zip: %w", f.Name, err)
		}
		data, err := readLimited(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("error reading binary from zip: %w", err)
		}
		return data, nil
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

	newPath := exePath + ".new"
	oldPath := exePath + ".old"

	// Write the new binary to a temp location in the same directory
	if err := os.WriteFile(newPath, data, mode); err != nil {
		return fmt.Errorf("failed to write new binary to %s: %w", newPath, err)
	}
	// os.WriteFile mode is subject to umask; enforce the intended permissions.
	if err := os.Chmod(newPath, mode); err != nil {
		os.Remove(newPath)
		return fmt.Errorf("failed to set permissions on %s: %w", newPath, err)
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

	// Clean up the old binary. Best effort: on Windows the running executable
	// cannot be removed, leaving a .old file behind.
	os.Remove(oldPath)

	return nil
}

func stripV(version string) string {
	return strings.TrimPrefix(version, "v")
}

func parseVersion(s string) (major, minor, patch int, err error) {
	s = stripV(s)
	majorStr, rest, ok := strings.Cut(s, ".")
	if !ok {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", s)
	}
	minorStr, patchStr, ok := strings.Cut(rest, ".")
	if !ok {
		return 0, 0, 0, fmt.Errorf("invalid version format: %s", s)
	}

	major, err = strconv.Atoi(majorStr)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid major version: %w", err)
	}
	minor, err = strconv.Atoi(minorStr)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid minor version: %w", err)
	}

	// Handle pre-release suffixes like "1.0.0-pre" — take only the numeric part
	patchStr, _, _ = strings.Cut(patchStr, "-")
	patch, err = strconv.Atoi(patchStr)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid patch version: %w", err)
	}

	return major, minor, patch, nil
}

// ---------------------------------------------------------------------------
// Testing helpers
// ---------------------------------------------------------------------------

// SetHTTPClient replaces the package-level HTTP client.
// Intended for tests only; not safe to call concurrently with an in-flight
// check or download.
func SetHTTPClient(c *http.Client) {
	httpClient = c
}

// execPathFunc overrides the function used to determine the current
// executable path. Intended for tests; nil means the real path is used.
var execPathFunc func() (string, error)

func getExecPath() (string, error) {
	if execPathFunc != nil {
		return execPathFunc()
	}
	return executablePath()
}
