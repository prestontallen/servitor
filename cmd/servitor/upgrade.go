package main

import (
	"archive/tar"
	"bytes"
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
	"strings"
)

const upgradeRepo = "prestontallen/servitor"

type ghRelease struct {
	TagName string `json:"tag_name"`
}

func cmdUpgrade(stdout, stderr io.Writer) int {
	osname := runtime.GOOS
	arch := runtime.GOARCH
	if osname != "linux" && osname != "darwin" {
		fmt.Fprintln(stderr, "servitor: upgrade not supported on", osname)
		return 1
	}

	// 1. resolve latest release tag
	resp, err := http.Get("https://api.github.com/repos/" + upgradeRepo + "/releases/latest")
	if err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		fmt.Fprintf(stderr, "servitor: upgrade: github api %s\n", resp.Status)
		return 1
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}
	tag := rel.TagName
	if tag == "" {
		fmt.Fprintln(stderr, "servitor: upgrade: no release tag found")
		return 1
	}

	base := "https://github.com/" + upgradeRepo + "/releases/download/" + tag
	tarName := fmt.Sprintf("servitor_%s_%s.tar.gz", osname, arch)
	tarURL := base + "/" + tarName
	checkURL := base + "/checksums.txt"

	// temp dir
	tmp, err := os.MkdirTemp("", "servitor-upgrade-*")
	if err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	// download checksums
	checksums, err := fetchBytes(checkURL)
	if err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}
	expected := ""
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == tarName {
			expected = fields[0]
		}
	}
	if expected == "" {
		fmt.Fprintln(stderr, "servitor: upgrade: checksum not found for", tarName)
		return 1
	}

	// download tarball
	tarBytes, err := fetchBytes(tarURL)
	if err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}
	h := sha256.Sum256(tarBytes)
	if hex.EncodeToString(h[:]) != expected {
		fmt.Fprintln(stderr, "servitor: upgrade: checksum mismatch")
		return 1
	}

	// extract
	extractDir := filepath.Join(tmp, "extract")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}
	if err := extractTarGz(tarBytes, extractDir); err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}

	// find current binary dir
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(stderr, "servitor: upgrade:", err)
		return 1
	}
	exe, _ = filepath.EvalSymlinks(exe)
	binDir := filepath.Dir(exe)

	for _, bin := range []string{"servitor", "servitord", "servitor-mcp"} {
		src := filepath.Join(extractDir, bin)
		dst := filepath.Join(binDir, bin)
		if err := atomicReplace(src, dst); err != nil {
			fmt.Fprintf(stderr, "servitor: upgrade: replace %s: %v\n", bin, err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "upgraded to %s\n", tag)
	fmt.Fprintln(stdout, "restart servitord if running.")
	return 0
}

func fetchBytes(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func extractTarGz(data []byte, dest string) error {
	r := bytes.NewReader(data)
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
			os.Chmod(target, os.FileMode(hdr.Mode))
		}
	}
	return nil
}

func atomicReplace(src, dst string) error {
	// read src
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	tmp := dst + ".new"
	if err := os.WriteFile(tmp, data, 0755); err != nil {
		return err
	}
	// preserve mode
	if info, err := os.Stat(src); err == nil {
		os.Chmod(tmp, info.Mode())
	}
	return os.Rename(tmp, dst)
}
