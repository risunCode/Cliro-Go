package cloudflared

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/config"
	"cliro-go/internal/logger"
)

var (
	quickTunnelURLPattern = regexp.MustCompile(`https://[a-zA-Z0-9.-]+\.trycloudflare\.com`)
	namedTunnelURLPattern = regexp.MustCompile(`\\"hostname\\":\\"([^\\"]+)\\"`)
)

type Status struct {
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	Running   bool   `json:"running"`
	URL       string `json:"url,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Manager struct {
	mu       sync.RWMutex
	process  *exec.Cmd
	waitDone chan struct{}
	stopping bool
	status   Status
	binPath  string
	log      *logger.Logger
	client   *http.Client
}

func NewManager(dataDir string, log *logger.Logger) *Manager {
	binName := "cloudflared"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	return &Manager{
		binPath: filepath.Join(dataDir, "bin", binName),
		log:     log,
		client: &http.Client{
			Timeout: 10 * time.Minute,
		},
	}
}

func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) RefreshStatus() Status {
	installed, version := m.checkInstalled()

	m.mu.Lock()
	m.status.Installed = installed
	m.status.Version = version
	status := m.status
	m.mu.Unlock()

	return status
}

func (m *Manager) Install(ctx context.Context) (Status, error) {
	binDir := filepath.Dir(m.binPath)
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return m.RefreshStatus(), fmt.Errorf("create cloudflared bin dir: %w", err)
	}

	downloadURL, isArchive, err := downloadURL()
	if err != nil {
		return m.RefreshStatus(), err
	}
	if m.log != nil {
		m.log.Info("cloudflared", "downloading cloudflared from "+downloadURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return m.RefreshStatus(), fmt.Errorf("create cloudflared request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return m.RefreshStatus(), fmt.Errorf("download cloudflared: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return m.RefreshStatus(), fmt.Errorf("download cloudflared failed with status %d", resp.StatusCode)
	}

	if isArchive {
		if err := extractCloudflaredArchive(resp.Body, m.binPath); err != nil {
			return m.RefreshStatus(), err
		}
	} else {
		if err := writeCloudflaredBinary(resp.Body, m.binPath); err != nil {
			return m.RefreshStatus(), err
		}
	}

	if err := ensureExecutable(m.binPath); err != nil {
		return m.RefreshStatus(), err
	}

	status := m.RefreshStatus()
	if !status.Installed {
		return status, fmt.Errorf("cloudflared installation verification failed")
	}
	if m.log != nil {
		m.log.Info("cloudflared", "cloudflared installed successfully")
	}
	return status, nil
}

func (m *Manager) Start(settings config.CloudflaredSettings, port int) (Status, error) {
	if port <= 0 {
		return m.RefreshStatus(), fmt.Errorf("invalid proxy port")
	}

	m.mu.RLock()
	if m.process != nil {
		status := m.status
		m.mu.RUnlock()
		return status, nil
	}
	m.mu.RUnlock()

	installed, version := m.checkInstalled()
	if !installed {
		status := m.RefreshStatus()
		return status, fmt.Errorf("cloudflared is not installed")
	}

	localURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	args := []string{"tunnel"}
	switch settings.Mode {
	case config.CloudflaredModeAuth:
		if strings.TrimSpace(settings.Token) == "" {
			return m.RefreshStatus(), fmt.Errorf("tunnel token is required for named tunnel mode")
		}
		args = append(args, "run", "--token", strings.TrimSpace(settings.Token))
	default:
		args = append(args, "--url", localURL)
	}
	if settings.UseHTTP2 {
		args = append(args, "--protocol", "http2")
	}

	cmd := exec.Command(m.binPath, args...)
	if dir := filepath.Dir(m.binPath); dir != "" {
		cmd.Dir = dir
	}
	configureCommand(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return m.RefreshStatus(), fmt.Errorf("attach cloudflared stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return m.RefreshStatus(), fmt.Errorf("attach cloudflared stderr: %w", err)
	}

	if err := cmd.Start(); err != nil {
		m.updateError(fmt.Sprintf("failed to start cloudflared: %v", err))
		return m.RefreshStatus(), fmt.Errorf("start cloudflared: %w", err)
	}

	done := make(chan struct{})
	m.mu.Lock()
	m.process = cmd
	m.waitDone = done
	m.stopping = false
	m.status.Installed = installed
	m.status.Version = version
	m.status.Running = true
	m.status.URL = ""
	m.status.Error = ""
	status := m.status
	m.mu.Unlock()

	m.consumeOutput(stdout)
	m.consumeOutput(stderr)
	go m.waitForExit(cmd, done)

	if m.log != nil {
		m.log.Info("cloudflared", fmt.Sprintf("started cloudflared tunnel mode=%s url=%s", settings.Mode, localURL))
	}
	return status, nil
}

func (m *Manager) Stop() (Status, error) {
	m.mu.RLock()
	cmd := m.process
	done := m.waitDone
	m.mu.RUnlock()

	if cmd != nil && cmd.Process != nil {
		m.mu.Lock()
		m.stopping = true
		m.mu.Unlock()
		if err := cmd.Process.Kill(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "finished") {
			return m.RefreshStatus(), fmt.Errorf("stop cloudflared: %w", err)
		}
		if done != nil {
			select {
			case <-done:
			case <-time.After(5 * time.Second):
			}
		}
	}

	m.mu.Lock()
	m.process = nil
	m.waitDone = nil
	m.stopping = false
	m.status.Running = false
	m.status.URL = ""
	m.status.Error = ""
	status := m.status
	m.mu.Unlock()

	if m.log != nil {
		m.log.Info("cloudflared", "stopped cloudflared tunnel")
	}
	return status, nil
}

func (m *Manager) Shutdown() {
	_, _ = m.Stop()
}

func (m *Manager) updateError(message string) {
	m.mu.Lock()
	m.status.Error = message
	m.mu.Unlock()
	if m.log != nil {
		m.log.Warn("cloudflared", message)
	}
}

func (m *Manager) consumeOutput(reader io.ReadCloser) {
	go func() {
		defer func() { _ = reader.Close() }()
		scanner := bufio.NewScanner(reader)
		buffer := make([]byte, 0, 64*1024)
		scanner.Buffer(buffer, 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if m.log != nil && strings.TrimSpace(line) != "" {
				m.log.Info("cloudflared", line)
			}
			if url := extractTunnelURL(line); url != "" {
				m.mu.Lock()
				m.status.URL = url
				m.status.Error = ""
				m.mu.Unlock()
			}
		}
		if err := scanner.Err(); err != nil && m.log != nil {
			m.log.Warn("cloudflared", "cloudflared output reader failed: "+err.Error())
		}
	}()
}

func (m *Manager) waitForExit(cmd *exec.Cmd, done chan struct{}) {
	err := cmd.Wait()
	close(done)

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process == cmd {
		m.process = nil
	}
	m.waitDone = nil
	expected := m.stopping
	m.stopping = false
	m.status.Running = false
	if expected {
		return
	}
	if err != nil {
		m.status.Error = fmt.Sprintf("tunnel process exited: %v", err)
		return
	}
	m.status.Error = "tunnel process exited"
}

func (m *Manager) checkInstalled() (bool, string) {
	info, err := os.Stat(m.binPath)
	if err != nil || info.IsDir() {
		return false, ""
	}

	cmd := exec.Command(m.binPath, "--version")
	configureCommand(cmd)
	output, err := cmd.Output()
	if err != nil {
		return false, ""
	}

	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return true, trimmed
		}
	}
	return true, ""
}

func downloadURL() (string, bool, error) {
	var platform string
	var arch string
	var ext string

	switch runtime.GOOS {
	case "windows":
		platform = "windows"
		ext = ".exe"
	case "linux":
		platform = "linux"
	case "darwin":
		platform = "darwin"
		ext = ".tgz"
	default:
		return "", false, fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", false, fmt.Errorf("unsupported architecture: %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	url := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-%s-%s%s", platform, arch, ext)
	return url, ext == ".tgz", nil
}

func writeCloudflaredBinary(reader io.Reader, binPath string) error {
	file, err := os.Create(binPath)
	if err != nil {
		return fmt.Errorf("create cloudflared binary: %w", err)
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("write cloudflared binary: %w", err)
	}
	return nil
}

func extractCloudflaredArchive(reader io.Reader, binPath string) error {
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return fmt.Errorf("open cloudflared archive: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read cloudflared archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(header.Name)
		if name != "cloudflared" && name != "cloudflared.exe" {
			continue
		}
		if err := writeCloudflaredBinary(tarReader, binPath); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("cloudflared binary not found in archive")
}

func ensureExecutable(binPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(binPath, 0o755); err != nil {
		return fmt.Errorf("set cloudflared permissions: %w", err)
	}
	return nil
}

func extractTunnelURL(line string) string {
	if match := quickTunnelURLPattern.FindString(line); match != "" {
		return match
	}
	if matches := namedTunnelURLPattern.FindStringSubmatch(line); len(matches) == 2 {
		hostname := strings.TrimSpace(matches[1])
		if hostname != "" {
			return "https://" + hostname
		}
	}
	return ""
}
