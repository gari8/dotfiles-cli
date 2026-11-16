package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

const releasesURL = "https://github.com/gari8/dotfiles-cli/releases"

var httpClient = &http.Client{Timeout: 60 * time.Second}

// latestRelease returns the newest released version (without the leading v),
// read from where /releases/latest redirects to.
func latestRelease() (string, error) {
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Head(releasesURL + "/latest")
	if err != nil {
		return "", fmt.Errorf("cannot check for a new dots: %w", err)
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	tag := loc[strings.LastIndex(loc, "/")+1:]
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("cannot check for a new dots: unexpected redirect %q", loc)
	}
	return strings.TrimPrefix(tag, "v"), nil
}

// parseVersion reads 1.2.3 (a -suffix is ignored). ok is false for builds
// without a version, such as "dev".
func parseVersion(v string) (parts [3]int, ok bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}

// newerVersion reports whether latest is newer than current.
func newerVersion(current, latest string) bool {
	c, ok1 := parseVersion(current)
	l, ok2 := parseVersion(latest)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

// selfPath returns the running dots binary, or an error when it was installed
// some other way than install.sh and should be updated that way.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	managed := []struct{ part, how string }{
		{"/go/bin/", "go install github.com/gari8/dotfiles-cli/cmd/dots@latest"},
		{"/mise/installs/", "mise upgrade"},
		{"/Cellar/", "brew upgrade"},
	}
	for _, m := range managed {
		if strings.Contains(exe, m.part) {
			return "", fmt.Errorf("%s was not installed by install.sh; update it with: %s", tildePath(exe), m.how)
		}
	}
	if gobin := os.Getenv("GOBIN"); gobin != "" && filepath.Dir(exe) == filepath.Clean(gobin) {
		return "", fmt.Errorf("%s was installed by go install; update it with: go install github.com/gari8/dotfiles-cli/cmd/dots@latest", tildePath(exe))
	}
	return exe, nil
}

// selfUpdate replaces the running binary with release v<latest>, after
// checking the archive against checksums.txt.
func selfUpdate(latest string) error {
	exe, err := selfPath()
	if err != nil {
		return err
	}
	archive := fmt.Sprintf("dotfiles-cli_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
	base := fmt.Sprintf("%s/download/v%s/", releasesURL, latest)
	fmt.Printf("  download %s\n", archive)

	data, err := download(base + archive)
	if err != nil {
		return err
	}
	sums, err := download(base + "checksums.txt")
	if err != nil {
		return err
	}
	if err := checkSum(data, string(sums), archive); err != nil {
		return err
	}
	bin, err := extractDots(data)
	if err != nil {
		return err
	}

	// Write next to the binary and rename over it, so a failure leaves the
	// old one in place and the running process is not affected.
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".dots-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", tildePath(filepath.Dir(exe)), err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		return err
	}
	fmt.Printf("  updated %s to %s\n", tildePath(exe), latest)
	return nil
}

func download(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// checkSum verifies data against the line for name in a checksums.txt.
func checkSum(data []byte, sums, name string) error {
	for _, line := range strings.Split(sums, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == name {
			got := sha256.Sum256(data)
			if hex.EncodeToString(got[:]) != f[0] {
				return fmt.Errorf("checksum mismatch for %s", name)
			}
			return nil
		}
	}
	return fmt.Errorf("%s is not in checksums.txt", name)
}

// extractDots returns the dots binary from a release archive.
func extractDots(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, errors.New("dots is not in the release archive")
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(h.Name) == "dots" && h.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
}
