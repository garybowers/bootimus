package smb

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
)

type Manager struct {
	dataDir string
	addr    string
	port    int

	mu     sync.RWMutex
	shares map[string]string

	cmd *exec.Cmd
}

func NewManager(dataDir string, addr string, port int) *Manager {
	return &Manager{
		dataDir: dataDir,
		addr:    addr,
		port:    port,
		shares:  make(map[string]string),
	}
}

func (m *Manager) AddShare(name, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shares[name] = path
}

func (m *Manager) RemoveShare(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.shares, name)
}

func (m *Manager) HasShare(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.shares[name]
	return ok
}

func (m *Manager) ShareCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.shares)
}

func (m *Manager) Port() int {
	return m.port
}

func (m *Manager) Start() error {
	smbdPath, err := exec.LookPath("smbd")
	if err != nil {
		return fmt.Errorf("smbd not found in PATH (install the samba package): %w", err)
	}

	if err := m.ensureStateDirs(); err != nil {
		return fmt.Errorf("failed to create smbd state directories: %w", err)
	}

	if err := m.writeConfig(); err != nil {
		return fmt.Errorf("failed to write smb.conf: %w", err)
	}

	m.cmd = exec.Command(smbdPath, "--no-process-group", "--foreground", "--configfile", m.configPath())
	m.cmd.Stdout = os.Stdout
	m.cmd.Stderr = os.Stderr

	if err := m.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start smbd: %w", err)
	}

	log.Printf("SMB: smbd started (PID %d, port %d)", m.cmd.Process.Pid, m.port)

	go func(cmd *exec.Cmd) {
		err := cmd.Wait()
		if err != nil {
			log.Printf("SMB: smbd exited: %v (check %s/log/smbd.log)", err, m.smbDir())
		} else {
			log.Printf("SMB: smbd exited cleanly")
		}
	}(m.cmd)

	return nil
}

func (m *Manager) Reload() error {
	if err := m.writeConfig(); err != nil {
		return fmt.Errorf("failed to write smb.conf: %w", err)
	}
	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}
	ctrlPath, err := exec.LookPath("smbcontrol")
	if err != nil {
		log.Printf("SMB: warning - smbcontrol not found, cannot reload smbd: %v", err)
		return nil
	}
	out, cErr := exec.Command(ctrlPath, "--configfile", m.configPath(), "smbd", "reload-config").CombinedOutput()
	if cErr != nil {
		log.Printf("SMB: warning - smbcontrol reload-config failed: %v (%s)", cErr, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) Stop() {
	if m.cmd != nil && m.cmd.Process != nil {
		if err := m.cmd.Process.Kill(); err != nil {
			log.Printf("SMB: warning - could not kill smbd: %v", err)
		}
	}
}

func SanitiseShareName(isoBase string) string {
	var sb strings.Builder
	for _, r := range isoBase {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			sb.WriteRune(r)
		default:
			sb.WriteRune('_')
		}
	}
	result := sb.String()
	if len(result) > 80 {
		result = result[:80]
	}
	return result
}

func (m *Manager) smbDir() string     { return filepath.Join(m.dataDir, "smb") }
func (m *Manager) configPath() string { return filepath.Join(m.smbDir(), "smb.conf") }

func (m *Manager) ensureStateDirs() error {
	for _, sub := range []string{"locks", "state", "cache", "pid", "log", "ncalrpc", "private", "usershares"} {
		if err := os.MkdirAll(filepath.Join(m.smbDir(), sub), 0755); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) writeConfig() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	configTemplate := template.Must(template.New("config").Parse(`[global]
workgroup = WORKGROUP
server role = standalone server
log level = 1
log file = {{ .Dir }}/log/smbd.log
bind interfaces only = yes
interfaces = {{ .Addr }}
smb ports = {{ .Port }}
server min protocol = SMB2
map to guest = bad user
guest account = nobody
load printers = no
disable netbios = yes
disable spoolss = yes
# Install clients (WinPE) reboot mid-session and reconnect with the same
# IP. Without these, smbd hangs onto the prior tree connect/oplocks and
# the next net use fails. Locks aren't meaningful for a read-only share.
oplocks = no
kernel oplocks = no
level2 oplocks = no
strict locking = no
deadtime = 1
# Windows sends VC=0 on session setup after a reboot. Without this, smbd
# keeps the prior session from the same client IP alive and refuses the
# new one. This is the specific fix for "net use fails after VM reboot".
reset on zero vc = yes
lock directory = {{ .Dir }}/locks
state directory = {{ .Dir }}/state
cache directory = {{ .Dir }}/cache
pid directory = {{ .Dir }}/pid
ncalrpc dir = {{ .Dir }}/ncalrpc
private dir = {{ .Dir }}/private
usershare path = {{ .Dir }}/usershares
acl allow execute always = yes

{{- range $name, $path := .Shares }}

[{{ $name }}]
path = {{ $path }}
read only = yes
guest ok = yes
# Guest maps to "nobody" (uid 65534), which NFS-backed data dirs commonly
# reject (root squash / export rules only trust specific UIDs). The
# bootimus process already reads these files as root, so let smbd do the
# same. Shares are read-only, so this only widens reads.
force user = root
browseable = yes

{{- end }}
`))

	var buf bytes.Buffer
	err := configTemplate.Execute(&buf, &struct {
		Dir    string
		Addr   string
		Port   int
		Shares map[string]string
	}{
		Dir:    m.smbDir(),
		Addr:   m.addr,
		Port:   m.port,
		Shares: m.shares,
	})
	if err != nil {
		return err
	}

	return os.WriteFile(m.configPath(), buf.Bytes(), 0644)
}
