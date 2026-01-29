package update

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// GoInstall installs via `go install github.com/max-pantom/daily/cmd/daily@version`.
func GoInstall(version string, stdout, stderr io.Writer) error {
	cmd := exec.Command("go", "install", "github.com/max-pantom/daily/cmd/daily@"+version)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// BinaryInstall downloads a release tarball from GitHub and installs the binary to dest.
// Asset naming convention: daily_<tag>_<os>_<arch>.tar.gz (e.g., daily_v0.1.3_darwin_arm64.tar.gz).
// If version == "latest", it resolves the latest release tag via GitHub API.
func BinaryInstall(version, dest string, stdout, stderr io.Writer) error {
	if version == "" {
		version = "latest"
	}
	if version == "latest" {
		v, err := latestTag()
		if err != nil {
			return err
		}
		version = v
	}

	osName, archName, err := platform()
	if err != nil {
		return err
	}

	asset := fmt.Sprintf("daily_%s_%s_%s.tar.gz", version, osName, archName)
	url := fmt.Sprintf("https://github.com/max-pantom/daily/releases/download/%s/%s", version, asset)

	tmp, err := os.CreateTemp("", "daily_dl_*.tar.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, httpResp(url)); err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}

	binPath, err := untarSingle(tmp, "daily")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := copyFile(binPath, dest); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "installed daily %s to %s\n", version, dest)
	return nil
}

func platform() (string, string, error) {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	switch osName {
	case "darwin", "linux":
	default:
		return "", "", fmt.Errorf("unsupported os: %s", osName)
	}
	switch arch {
	case "amd64", "arm64":
	default:
		return "", "", fmt.Errorf("unsupported arch: %s", arch)
	}
	return osName, arch, nil
}

func latestTag() (string, error) {
	url := "https://api.github.com/repos/max-pantom/daily/releases/latest"
	resp := httpResp(url)
	defer resp.Close()
	var data struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp).Decode(&data); err != nil {
		return "", err
	}
	if data.TagName == "" {
		return "", errors.New("latest tag not found")
	}
	return data.TagName, nil
}

func httpResp(url string) io.ReadCloser {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "daily-updater")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		panic(fmt.Errorf("download failed %s: %s", url, res.Status))
	}
	return res.Body
}

func untarSingle(r io.Reader, want string) (string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	tmpDir, err := os.MkdirTemp("", "daily_bin_*")
	if err != nil {
		return "", err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		name := filepath.Base(hdr.Name)
		if name != want {
			continue
		}
		outPath := filepath.Join(tmpDir, want)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		out.Close()
		return outPath, nil
	}
	return "", fmt.Errorf("binary %s not found in archive", want)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}
