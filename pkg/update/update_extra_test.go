package update

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExtractZipSkipsDirectoryEntry ensures a directory entry whose base name
// matches the binary is not extracted as a zero-byte binary.
func TestExtractZipSkipsDirectoryEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Directory entry named like the binary (trailing slash, dir mode).
	dirHdr := &zip.FileHeader{Name: "jc2aws/"}
	dirHdr.SetMode(os.ModeDir | 0755)
	if _, err := zw.CreateHeader(dirHdr); err != nil {
		t.Fatal(err)
	}

	// The real binary nested inside.
	fw, err := zw.Create("jc2aws/jc2aws")
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("#!/bin/sh\necho real binary\n")
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	data, err := extractZip(archivePath, "jc2aws")
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("extracted a zero-byte binary (directory entry matched)")
	}
	if !bytes.Equal(data, content) {
		t.Errorf("extracted content mismatch")
	}
}

// TestExtractZipOnlyDirectoryEntry ensures an archive holding only a matching
// directory entry errors out instead of returning empty data.
func TestExtractZipOnlyDirectoryEntry(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	dirHdr := &zip.FileHeader{Name: "jc2aws/"}
	dirHdr.SetMode(os.ModeDir | 0755)
	if _, err := zw.CreateHeader(dirHdr); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(t.TempDir(), "dironly.zip")
	if err := os.WriteFile(archivePath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := extractZip(archivePath, "jc2aws"); err == nil {
		t.Fatal("expected 'not found' error for directory-only archive, got nil")
	}
}

// ---------------------------------------------------------------------------
// parseChecksumFile format tolerance
// ---------------------------------------------------------------------------

func TestParseChecksumFileVariants(t *testing.T) {
	const hash = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"

	tests := []struct {
		name    string
		content string
		target  string
		wantErr bool
	}{
		{"plain", hash + "  file.tar.gz", "file.tar.gz", false},
		{"binary marker", hash + " *file.tar.gz", "file.tar.gz", false},
		{"path prefix", hash + "  dist/file.tar.gz", "file.tar.gz", false},
		{"missing", hash + "  other.tar.gz", "file.tar.gz", true},
		{"empty", "", "file.tar.gz", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseChecksumFile(strings.NewReader(tt.content), tt.target)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != hash {
				t.Errorf("hash = %q, want %q", got, hash)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseVersion strictness
// ---------------------------------------------------------------------------

func TestParseVersionRejectsTrailingComponents(t *testing.T) {
	if _, _, _, err := parseVersion("1.2.3.4"); err == nil {
		t.Error("expected error for 4-component version, got nil")
	}
	if _, _, _, err := parseVersion("latest"); err == nil {
		t.Error("expected error for non-numeric tag, got nil")
	}
}
