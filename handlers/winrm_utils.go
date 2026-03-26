package handlers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"

	"github.com/masterzen/winrm"
)

// WinRMConfig holds connection details for WinRM
type WinRMConfig struct {
	Host     string
	User     string
	Password string
	Port     int
	HTTPS    bool
	Insecure bool
	Timeout  time.Duration
}

// NewWinRMConfig creates a WinRM config with sensible defaults
func NewWinRMConfig(host, user, password string) *WinRMConfig {
	return &WinRMConfig{
		Host:     host,
		User:     user,
		Password: password,
		Port:     5985, // HTTP default; 5986 for HTTPS
		HTTPS:    false,
		Insecure: true,
		Timeout:  120 * time.Second,
	}
}

// newClient creates a WinRM client from config
func (c *WinRMConfig) newClient() (*winrm.Client, error) {
	endpoint := winrm.NewEndpoint(c.Host, c.Port, c.HTTPS, c.Insecure, nil, nil, nil, c.Timeout)
	client, err := winrm.NewClient(endpoint, c.User, c.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to create WinRM client for %s: %v", c.Host, err)
	}
	return client, nil
}

// CheckWinRMConnection validates WinRM connectivity to a Windows host
func CheckWinRMConnection(host, user, password string) error {
	cfg := NewWinRMConfig(host, user, password)
	client, err := cfg.newClient()
	if err != nil {
		return err
	}

	stdout, stderr, exitCode, err := RunWinRMCommand(client, "hostname")
	if err != nil {
		return fmt.Errorf("WinRM connection test failed: %v", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("WinRM connection test returned exit code %d: %s", exitCode, stderr)
	}

	log.Printf("WinRM connection successful to %s (hostname: %s)", host, strings.TrimSpace(stdout))
	return nil
}

// RunWinRMCommand executes a command (cmd.exe) on a remote Windows host via WinRM
func RunWinRMCommand(client *winrm.Client, command string) (stdout, stderr string, exitCode int, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer
	exitCode, err = client.Run(command, &stdoutBuf, &stderrBuf)
	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// RunWinRMPowerShell executes a PowerShell script on a remote Windows host via WinRM
func RunWinRMPowerShell(client *winrm.Client, script string) (stdout, stderr string, exitCode int, err error) {
	shell, err := client.CreateShell()
	if err != nil {
		return "", "", -1, fmt.Errorf("failed to create WinRM shell: %v", err)
	}
	defer shell.Close()

	// Wrap in powershell.exe -Command
	psCmd := fmt.Sprintf("powershell.exe -NoProfile -NonInteractive -Command \"%s\"", escapePowerShellForWinRM(script))

	var stdoutBuf, stderrBuf bytes.Buffer
	exitCode, err = client.Run(psCmd, &stdoutBuf, &stderrBuf)
	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// RunWinRMPowerShellScript executes a longer PowerShell script via WinRM
// Handles encoding for complex scripts that may contain special characters
func RunWinRMPowerShellScript(client *winrm.Client, script string) (stdout, stderr string, exitCode int, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	// Use PowerShell's encoded command for complex scripts
	encoded := winrm.Powershell(script)
	exitCode, err = client.Run(encoded, &stdoutBuf, &stderrBuf)
	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// escapePowerShellForWinRM escapes a PowerShell command for WinRM transport
func escapePowerShellForWinRM(script string) string {
	// Escape double quotes and backticks for the outer cmd.exe wrapping
	script = strings.ReplaceAll(script, `"`, `\"`)
	return script
}

// ----- High-level abstraction: SSH vs WinRM -----

// RemoteExecConfig determines how to connect to a remote host
type RemoteExecConfig struct {
	Host           string
	User           string
	Password       string
	ConnectionType string // "ssh" or "winrm"
}

// RunRemoteCommand runs a command on a remote host, choosing SSH or WinRM based on connection type
func RunRemoteCommand(cfg RemoteExecConfig, command string) (stdout string, err error) {
	switch cfg.ConnectionType {
	case "winrm":
		return runRemoteWinRM(cfg, command)
	default: // "ssh" or empty (backward compat)
		return runRemoteSSH(cfg, command)
	}
}

// RunRemotePowerShell runs a PowerShell script remotely — WinRM native, SSH via powershell.exe
func RunRemotePowerShell(cfg RemoteExecConfig, script string) (stdout string, err error) {
	switch cfg.ConnectionType {
	case "winrm":
		return runRemotePowerShellWinRM(cfg, script)
	default:
		return runRemotePowerShellSSH(cfg, script)
	}
}

// ----- Internal implementations -----

func runRemoteWinRM(cfg RemoteExecConfig, command string) (string, error) {
	winrmCfg := NewWinRMConfig(cfg.Host, cfg.User, cfg.Password)
	client, err := winrmCfg.newClient()
	if err != nil {
		return "", err
	}

	stdout, stderr, exitCode, err := RunWinRMCommand(client, command)
	if err != nil {
		return "", fmt.Errorf("WinRM command failed on %s: %v", cfg.Host, err)
	}
	if exitCode != 0 {
		return stdout, fmt.Errorf("WinRM command exited with code %d on %s: %s", exitCode, cfg.Host, stderr)
	}
	return stdout, nil
}

func runRemotePowerShellWinRM(cfg RemoteExecConfig, script string) (string, error) {
	winrmCfg := NewWinRMConfig(cfg.Host, cfg.User, cfg.Password)
	client, err := winrmCfg.newClient()
	if err != nil {
		return "", err
	}

	stdout, stderr, exitCode, err := RunWinRMPowerShellScript(client, script)
	if err != nil {
		return "", fmt.Errorf("WinRM PowerShell failed on %s: %v", cfg.Host, err)
	}
	if exitCode != 0 {
		return stdout, fmt.Errorf("WinRM PowerShell exited %d on %s: %s", exitCode, cfg.Host, stderr)
	}
	return stdout, nil
}

func runRemoteSSH(cfg RemoteExecConfig, command string) (string, error) {
	client, err := connectToServer(cfg.Host, cfg.User, cfg.Password)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed to %s: %v", cfg.Host, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session creation failed: %v", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	if err := session.Run(command); err != nil {
		return stdoutBuf.String(), fmt.Errorf("SSH command failed on %s: %v (%s)", cfg.Host, err, stderrBuf.String())
	}
	return stdoutBuf.String(), nil
}

func runRemotePowerShellSSH(cfg RemoteExecConfig, script string) (string, error) {
	client, err := connectToServer(cfg.Host, cfg.User, cfg.Password)
	if err != nil {
		return "", fmt.Errorf("SSH connection failed to %s: %v", cfg.Host, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session creation failed: %v", err)
	}
	defer session.Close()

	session.Stdin = strings.NewReader(script)
	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	if err := session.Run("powershell.exe"); err != nil {
		return stdoutBuf.String(), fmt.Errorf("SSH PowerShell failed on %s: %v (%s)", cfg.Host, err, stderrBuf.String())
	}
	return stdoutBuf.String(), nil
}

// ----- exec.Command-compatible wrappers for drop-in replacement -----

// ExecRemoteCommand replaces sshpass+ssh pattern. Returns combined output and error.
// This is a drop-in replacement for:
//
//	exec.Command("sshpass", "-p", pass, "ssh", "-o", "StrictHostKeyChecking=no", "user@host", "command")
func ExecRemoteCommand(cfg RemoteExecConfig, command string) ([]byte, error) {
	switch cfg.ConnectionType {
	case "winrm":
		winrmCfg := NewWinRMConfig(cfg.Host, cfg.User, cfg.Password)
		client, err := winrmCfg.newClient()
		if err != nil {
			return nil, err
		}
		stdout, stderr, exitCode, err := RunWinRMCommand(client, command)
		combined := stdout + stderr
		if err != nil {
			return []byte(combined), err
		}
		if exitCode != 0 {
			return []byte(combined), fmt.Errorf("exit code %d", exitCode)
		}
		return []byte(combined), nil
	default:
		cmd := exec.Command("sshpass", "-p", cfg.Password, "ssh", "-o", "StrictHostKeyChecking=no",
			fmt.Sprintf("%s@%s", cfg.User, cfg.Host), command)
		return cmd.CombinedOutput()
	}
}

// ExecRemotePowerShellCmd replaces sshpass+ssh+powershell.exe pattern. Returns combined output and error.
// This is a drop-in replacement for:
//
//	exec.Command("sshpass", "-p", pass, "ssh", "user@host", "powershell.exe", "-Command", script)
func ExecRemotePowerShellCmd(cfg RemoteExecConfig, script string) ([]byte, error) {
	switch cfg.ConnectionType {
	case "winrm":
		winrmCfg := NewWinRMConfig(cfg.Host, cfg.User, cfg.Password)
		client, err := winrmCfg.newClient()
		if err != nil {
			return nil, err
		}
		stdout, stderr, exitCode, err := RunWinRMPowerShellScript(client, script)
		combined := stdout + stderr
		if err != nil {
			return []byte(combined), err
		}
		if exitCode != 0 {
			return []byte(combined), fmt.Errorf("exit code %d: %s", exitCode, stderr)
		}
		return []byte(combined), nil
	default:
		cmd := exec.Command("sshpass", "-p", cfg.Password, "ssh", "-o", "StrictHostKeyChecking=no",
			fmt.Sprintf("%s@%s", cfg.User, cfg.Host), "powershell.exe", "-Command", script)
		return cmd.CombinedOutput()
	}
}

// CopyFileFromRemote copies a file from a remote host to local filesystem
// Drop-in replacement for sshpass+scp pattern
func CopyFileFromRemote(cfg RemoteExecConfig, remotePath, localPath string) error {
	return copyRemoteFileToLocalV2(cfg, remotePath, localPath)
}

// CopyFileToRemote copies a local file to a remote Windows host
func CopyFileToRemote(cfg RemoteExecConfig, localPath, remotePath string) error {
	switch cfg.ConnectionType {
	case "winrm":
		// Read local file, base64 encode, write remotely
		b64Cmd := fmt.Sprintf(`base64 -w0 '%s'`, localPath)
		b64Output, err := exec.Command("bash", "-c", b64Cmd).Output()
		if err != nil {
			return fmt.Errorf("failed to encode local file: %v", err)
		}
		script := fmt.Sprintf(`[IO.File]::WriteAllBytes('%s', [Convert]::FromBase64String('%s'))`, remotePath, strings.TrimSpace(string(b64Output)))
		_, err = RunRemotePowerShell(cfg, script)
		return err
	default:
		scpPath := strings.ReplaceAll(remotePath, `\`, "/")
		cmd := exec.Command("sshpass", "-p", cfg.Password, "scp", "-o", "StrictHostKeyChecking=no",
			localPath, fmt.Sprintf("%s@%s:%s", cfg.User, cfg.Host, scpPath))
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("SCP upload failed: %v (%s)", err, string(output))
		}
		return nil
	}
}

// GetDBRemoteConfig builds a RemoteExecConfig for the DB server of a given environment
func GetDBRemoteConfig(envName string) (RemoteExecConfig, error) {
	var host, connType, dbUser, dbPass, serverUser, serverPass string

	err := db.QueryRow(`SELECT app_dbhost, COALESCE(db_connection_type, 'ssh'),
		COALESCE(db_server_user, ''), COALESCE(db_server_password, ''),
		server_user, server_password
		FROM environments WHERE name = ?`, envName).Scan(&host, &connType, &dbUser, &dbPass, &serverUser, &serverPass)
	if err != nil {
		return RemoteExecConfig{}, fmt.Errorf("failed to get DB remote config for %s: %v", envName, err)
	}

	// For WinRM use the dedicated DB server credentials
	// For SSH fall back to the app server credentials (legacy behavior)
	if connType == "winrm" && dbUser != "" {
		return RemoteExecConfig{
			Host:           host,
			User:           dbUser,
			Password:       dbPass,
			ConnectionType: connType,
		}, nil
	}

	return RemoteExecConfig{
		Host:           host,
		User:           serverUser,
		Password:       serverPass,
		ConnectionType: connType,
	}, nil
}

// DetectSQLServerOSByConfig detects the OS of the SQL Server host using the appropriate connection method
func DetectSQLServerOSByConfig(cfg RemoteExecConfig) (string, error) {
	if cfg.ConnectionType == "winrm" {
		// WinRM is Windows-only
		log.Printf("Connection type is WinRM — OS is Windows")
		return "windows", nil
	}

	// SSH — try uname
	log.Println("Checking if SQL Server is running on Linux or Windows via SSH...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := connectToServer(cfg.Host, cfg.User, cfg.Password)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			log.Println("SSH timed out — assuming Windows")
			return "windows", nil
		}
		log.Printf("SSH failed — assuming Windows: %v", err)
		return "windows", nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "windows", nil
	}
	defer session.Close()

	var buf bytes.Buffer
	session.Stdout = &buf
	if err := session.Run("uname -s"); err != nil {
		return "windows", nil
	}

	output := strings.TrimSpace(buf.String())
	if strings.Contains(output, "Linux") {
		return "linux", nil
	}
	return "windows", nil
}

// BuildDBRemoteConfigFromHost looks up the connection type for a DB host from the environments table.
// This allows legacy function signatures (that pass host/user/pass) to automatically use WinRM when configured.
func BuildDBRemoteConfigFromHost(dbHost, fallbackUser, fallbackPass string) RemoteExecConfig {
	var connType, dbUser, dbPass string
	err := db.QueryRow(`SELECT COALESCE(db_connection_type, 'ssh'), COALESCE(db_server_user, ''), COALESCE(db_server_password, '')
		FROM environments WHERE app_dbhost = ? OR eazybi_dbhost = ? LIMIT 1`, dbHost, dbHost).Scan(&connType, &dbUser, &dbPass)
	if err != nil || connType == "" {
		connType = "ssh"
	}
	if connType == "winrm" && dbUser != "" {
		return RemoteExecConfig{Host: dbHost, User: dbUser, Password: dbPass, ConnectionType: "winrm"}
	}
	return RemoteExecConfig{Host: dbHost, User: fallbackUser, Password: fallbackPass, ConnectionType: "ssh"}
}
