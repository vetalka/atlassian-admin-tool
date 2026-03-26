package handlers

import (
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "github.com/microsoft/go-mssqldb"
)

// BackupDatabase handles backup - legacy signature, defaults to SSH
func BackupDatabase(dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, backupDir, serverUser, serverPassword string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	return BackupDatabaseWithConfig(cfg, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, backupDir)
}

// BackupDatabaseForEnv looks up the environment's DB connection config and performs backup
func BackupDatabaseForEnv(envName, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, backupDir string) error {
	cfg, err := GetDBRemoteConfig(envName)
	if err != nil {
		log.Printf("Failed to get DB remote config, falling back to SSH: %v", err)
		var serverUser, serverPass string
		db.QueryRow("SELECT server_user, server_password FROM environments WHERE name = ?", envName).Scan(&serverUser, &serverPass)
		cfg = RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPass, ConnectionType: "ssh"}
	}
	return BackupDatabaseWithConfig(cfg, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, backupDir)
}

// BackupDatabaseWithConfig performs backup using the specified remote execution config
func BackupDatabaseWithConfig(cfg RemoteExecConfig, dbHost, dbUser, dbPass, dbName, dbPort, dbDriver, backupDir string) error {
	log.Printf("Starting backup for database: %s with driver: %s (connection: %s)", dbName, dbDriver, cfg.ConnectionType)

	switch dbDriver {
	case "org.postgresql.Driver":
		return backupPostgreSQL(dbHost, dbUser, dbPass, dbName, dbPort, backupDir)
	case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
		return backupSQLServerSmart(cfg, dbHost, dbUser, dbPass, dbName, dbPort, backupDir)
	default:
		return fmt.Errorf("unsupported database type: %s", dbDriver)
	}
}

// backupSQLServerSmart picks the best backup strategy for SQL Server
// backupSQLServerSmart picks the best backup strategy for SQL Server
// backupSQLServerSmart picks the best backup strategy for SQL Server.
// Production flow: Direct SQL BACKUP → copy .bak via WinRM (or SSH for Linux SQL).
// If DB is on localhost, copies directly from the local filesystem.
func backupSQLServerSmart(cfg RemoteExecConfig, dbHost, dbUser, dbPass, dbName, dbPort, backupDir string) error {
	log.Printf("Attempting direct SQL Server backup for %s on %s:%s", dbName, dbHost, dbPort)

	backupFile := filepath.Join(backupDir, dbName+".bak")

	// Strategy 1: Direct SQL connection (just needs SQL port 1433 — works everywhere)
	remotePath, err := backupSQLServerDirect(dbHost, dbPort, dbUser, dbPass, dbName)
	if err != nil {
		log.Printf("Direct SQL backup failed: %v — trying remote shell approach", err)

		// Strategy 2: Fall back to remote shell (WinRM or SSH)
		osType, _ := DetectSQLServerOSByConfig(cfg)
		if osType == "linux" {
			return backupSQLServerLinux(dbHost, dbUser, dbPass, dbName, dbPort, backupDir, cfg.User, cfg.Password)
		}
		return backupSQLServerWindowsV2(cfg, dbUser, dbPass, dbName, dbPort, backupDir)
	}

	// BACKUP DATABASE succeeded — now copy the .bak file to the local backup directory
	log.Printf("BACKUP DATABASE succeeded at %s, copying to local...", remotePath)

	copyErr := copyBackupFileFromSQLServer(cfg, dbHost, dbPort, dbUser, dbPass, remotePath, backupFile)
	if copyErr != nil {
		return fmt.Errorf("backup completed on SQL Server at %s but file copy failed: %v", remotePath, copyErr)
	}

	log.Printf("SQL Server backup completed successfully: %s", backupFile)
	return nil
}

// copyBackupFileFromSQLServer tries multiple methods to copy a .bak file from the SQL Server.
// Order: local filesystem (if localhost) → WinRM/SSH → SQL-based transfer (via BCP).
func copyBackupFileFromSQLServer(cfg RemoteExecConfig, dbHost, dbPort, dbUser, dbPass, remotePath, localPath string) error {
	var copyErr error

	// Method 1: If DB is on localhost, try local filesystem copy
	if isLocalHost(dbHost) {
		copyErr = copyLocalSQLBackup(remotePath, localPath)
		if copyErr == nil {
			return nil
		}
		log.Printf("Local filesystem copy failed: %v", copyErr)
	}

	// Method 2: WinRM or SSH (the production path for remote SQL Servers)
	copyErr = copyRemoteFileToLocalV2(cfg, remotePath, localPath)
	if copyErr == nil {
		deleteRemoteFileV2(cfg, remotePath)
		return nil
	}
	log.Printf("Remote copy via %s failed: %v", cfg.ConnectionType, copyErr)

	// Method 3: WSL mount fallback — if running on WSL and the path is a Windows path,
	// try accessing via /mnt/c/... regardless of whether the host is "localhost"
	if looksLikeWindowsPath(remotePath) {
		log.Printf("Trying WSL mount fallback for Windows path: %s", remotePath)
		wslErr := copyLocalSQLBackup(remotePath, localPath)
		if wslErr == nil {
			log.Printf("WSL mount copy succeeded")
			return nil
		}
		log.Printf("WSL mount fallback failed: %v — trying SQL-based methods", wslErr)
	}

	// Method 4: SQL-based file transfer (needs server permissions)
	copyErr = copyFileViaBCP(dbHost, dbPort, dbUser, dbPass, remotePath, localPath, cfg)
	if copyErr == nil {
		return nil
	}
	log.Printf("SQL-based copy also failed: %v", copyErr)

	return fmt.Errorf("all copy methods failed (last error: %v). "+
		"For remote Windows SQL Server: ensure WinRM is enabled (run 'Enable-PSRemoting -Force' on the server "+
		"and open firewall port 5985)", copyErr)
}

// isLocalHost checks if a host address refers to the local machine
func isLocalHost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0"
}

// looksLikeWindowsPath returns true if the path looks like a Windows filesystem path
func looksLikeWindowsPath(path string) bool {
	return len(path) >= 2 && path[1] == ':' && (path[0] >= 'A' && path[0] <= 'Z' || path[0] >= 'a' && path[0] <= 'z')
}

// copyLocalSQLBackup copies a .bak file when SQL Server is on the local machine.
// Handles both native Linux and WSL environments.
func copyLocalSQLBackup(windowsPath, localPath string) error {
	// Try direct path first (works if path is already a Linux path or shared mount)
	if _, err := os.Stat(windowsPath); err == nil {
		log.Printf("Found backup file at direct path: %s", windowsPath)
		return copyFileLocal(windowsPath, localPath)
	}

	// Try WSL mount: C:ooar -> /mnt/c/foo/bar
	wslPath := windowsPathToLinux(windowsPath)
	if wslPath != "" {
		if _, err := os.Stat(wslPath); err == nil {
			log.Printf("Found backup file via WSL mount: %s", wslPath)
			return copyFileLocal(wslPath, localPath)
		}
	}

	return fmt.Errorf("backup file not accessible locally at %s (also tried %s)", windowsPath, wslPath)
}

// copyFileLocal copies a file from src to dst using standard file I/O
func copyFileLocal(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %v", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %v", err)
	}
	defer dstFile.Close()

	written, err := io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("copy failed: %v", err)
	}

	log.Printf("Local file copy completed: %d bytes -> %s", written, dst)
	return nil
}

// windowsPathToLinux converts a Windows path to a Linux/WSL accessible path.
// C:\foo\bar -> /mnt/c/foo/bar
func windowsPathToLinux(winPath string) string {
	p := strings.ReplaceAll(winPath, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		drive := strings.ToLower(string(p[0]))
		return "/mnt/" + drive + p[2:]
	}
	return ""
}

// copyFileViaBCP uses SQL Server's BCP or PowerShell via WinRM to stream
// a backup file. This works when SQL port is open but WinRM times out —
// it tells SQL Server to push the file out via a shared path or creates
// a temp table with the binary data.
func copyFileViaBCP(dbHost, dbPort, dbUser, dbPass, remotePath, localPath string, cfg RemoteExecConfig) error {
	connStr := fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=master;connection timeout=30;encrypt=disable",
		dbHost, dbPort, dbUser, dbPass)

	sqlDB, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return fmt.Errorf("SQL connection failed: %v", err)
	}
	defer sqlDB.Close()

	// Method A: Try OPENROWSET BULK (needs ADMINISTER BULK OPERATIONS permission)
	log.Printf("Trying OPENROWSET BULK read for %s", remotePath)
	query := fmt.Sprintf("SELECT BulkColumn FROM OPENROWSET(BULK N'%s', SINGLE_BLOB) AS x", remotePath)
	var data []byte
	err = sqlDB.QueryRow(query).Scan(&data)
	if err == nil && len(data) > 0 {
		if writeErr := os.WriteFile(localPath, data, 0644); writeErr != nil {
			return fmt.Errorf("failed to write local file: %v", writeErr)
		}
		log.Printf("OPENROWSET BULK transfer completed: %d bytes -> %s", len(data), localPath)
		return nil
	}
	log.Printf("OPENROWSET failed: %v — trying xp_cmdshell + certutil", err)

	// Method B: xp_cmdshell + certutil base64 encode (needs xp_cmdshell enabled)
	_, _ = sqlDB.Exec("EXEC sp_configure 'show advanced options', 1; RECONFIGURE")
	_, _ = sqlDB.Exec("EXEC sp_configure 'xp_cmdshell', 1; RECONFIGURE")

	tempB64 := strings.ReplaceAll(remotePath, ".bak", "_b64.txt")
	encodeCmd := fmt.Sprintf(`EXEC xp_cmdshell 'certutil -encode "%s" "%s"'`, remotePath, tempB64)
	_, err = sqlDB.Exec(encodeCmd)
	if err != nil {
		return fmt.Errorf("all SQL-based copy methods failed (OPENROWSET and xp_cmdshell unavailable)")
	}

	// Read the base64 content
	readCmd := fmt.Sprintf(`EXEC xp_cmdshell 'type "%s"'`, tempB64)
	rows, err := sqlDB.Query(readCmd)
	if err != nil {
		return fmt.Errorf("failed to read base64 file: %v", err)
	}
	defer rows.Close()

	var b64Content strings.Builder
	for rows.Next() {
		var line sql.NullString
		if scanErr := rows.Scan(&line); scanErr != nil || !line.Valid {
			continue
		}
		trimmed := strings.TrimSpace(line.String)
		if trimmed == "" || strings.HasPrefix(trimmed, "-----") || strings.Contains(trimmed, "CertUtil") {
			continue
		}
		b64Content.WriteString(trimmed)
	}

	// Cleanup
	sqlDB.Exec(fmt.Sprintf(`EXEC xp_cmdshell 'del "%s"'`, tempB64))

	if b64Content.Len() == 0 {
		return fmt.Errorf("certutil produced empty output")
	}

	decoded, err := base64.StdEncoding.DecodeString(b64Content.String())
	if err != nil {
		return fmt.Errorf("base64 decode failed: %v", err)
	}

	if writeErr := os.WriteFile(localPath, decoded, 0644); writeErr != nil {
		return fmt.Errorf("failed to write local file: %v", writeErr)
	}

	log.Printf("Certutil/base64 transfer completed: %d bytes -> %s", len(decoded), localPath)
	return nil
}

// backupSQLServerDirect uses Go's database/sql to run BACKUP DATABASE directly.
// Returns the remote path where the .bak file was written.
func backupSQLServerDirect(dbHost, dbPort, dbUser, dbPass, dbName string) (string, error) {
	connStr := fmt.Sprintf("server=%s;port=%s;user id=%s;password=%s;database=master;connection timeout=30;encrypt=disable",
		dbHost, dbPort, dbUser, dbPass)

	sqlDB, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return "", fmt.Errorf("failed to open SQL Server connection: %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return "", fmt.Errorf("SQL Server ping failed: %v", err)
	}
	log.Printf("Connected to SQL Server %s:%s, executing BACKUP DATABASE...", dbHost, dbPort)

	// Build ordered list of candidate backup paths (best to worst)
	candidates := []string{}

	// 1. SQL Server's default backup directory (service account always has write access)
	var defaultDir sql.NullString
	_ = sqlDB.QueryRow(`
		DECLARE @path NVARCHAR(4000)
		EXEC master.dbo.xp_instance_regread N'HKEY_LOCAL_MACHINE',
			N'Software\Microsoft\MSSQLServer\MSSQLServer', N'BackupDirectory', @path OUTPUT
		SELECT @path`).Scan(&defaultDir)
	if defaultDir.Valid && defaultDir.String != "" {
		candidates = append(candidates, fmt.Sprintf(`%s\%s.bak`, defaultDir.String, dbName))
		log.Printf("SQL Server default backup dir: %s", defaultDir.String)
	}

	// 2. SQL Server data directory
	var dataDir sql.NullString
	_ = sqlDB.QueryRow("SELECT CAST(SERVERPROPERTY('InstanceDefaultDataPath') AS NVARCHAR(4000))").Scan(&dataDir)
	if dataDir.Valid && dataDir.String != "" {
		candidates = append(candidates, fmt.Sprintf(`%s%s.bak`, dataDir.String, dbName))
	}

	// 3. Fallback
	candidates = append(candidates, fmt.Sprintf(`C:	emp\%s.bak`, dbName))

	for _, path := range candidates {
		log.Printf("Trying BACKUP DATABASE to: %s", path)
		backupSQL := fmt.Sprintf("BACKUP DATABASE [%s] TO DISK = N'%s' WITH FORMAT, INIT, SKIP, NOREWIND, NOUNLOAD", dbName, path)
		_, execErr := sqlDB.Exec(backupSQL)
		if execErr == nil {
			log.Printf("BACKUP DATABASE succeeded: %s -> %s", dbName, path)
			return path, nil
		}
		log.Printf("BACKUP to %s failed: %v", path, execErr)
	}

	return "", fmt.Errorf("BACKUP DATABASE failed for all candidate paths on %s:%s", dbHost, dbPort)
}

func backupPostgreSQL(dbHost, dbUser, dbPass, dbName, dbPort, backupDir string) error {
	backupFilePath := filepath.Join(backupDir, dbName+".sql")
	cmd := exec.Command("pg_dump", "-h", dbHost, "-U", dbUser, "-d", dbName, "-p", dbPort, "-f", backupFilePath)
	cmd.Env = append(cmd.Env, fmt.Sprintf("PGPASSWORD=%s", dbPass))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("PostgreSQL backup failed: %v (%s)", err, string(output))
	}
	log.Printf("PostgreSQL backup completed: %s", backupFilePath)
	return nil
}

func backupSQLServerLinux(dbHost, dbUser, dbPass, dbName, dbPort, backupDir, serverUser, serverPassword string) error {
	tempPath := fmt.Sprintf("/tmp/%s.bak", dbName)
	backupFile := filepath.Join(backupDir, dbName+".bak")
	sqlCmd := fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -Q "BACKUP DATABASE [%s] TO DISK = N'%s'"`, dbHost, dbPort, serverUser, serverPassword, dbName, tempPath)
	if output, err := exec.Command("bash", "-c", sqlCmd).CombinedOutput(); err != nil {
		return fmt.Errorf("SQL Server Linux backup failed: %v (%s)", err, string(output))
	}
	if output, err := exec.Command("scp", fmt.Sprintf("%s@%s:%s", dbUser, dbHost, tempPath), backupFile).CombinedOutput(); err != nil {
		return fmt.Errorf("SCP copy failed: %v (%s)", err, string(output))
	}
	log.Printf("SQL Server Linux backup completed: %s", backupFile)
	return nil
}

// ============== Windows SQL Server Backup (WinRM or SSH) ==============

func backupSQLServerWindowsV2(cfg RemoteExecConfig, dbUser, dbPass, dbName, dbPort, backupDir string) error {
	tempPath := fmt.Sprintf(`C:\temp\%s.bak`, dbName)
	backupFile := filepath.Join(backupDir, dbName+".bak")

	log.Printf("Starting Windows SQL Server backup for %s via %s", dbName, cfg.ConnectionType)

	// Validate connection
	if err := validateSQLServerConnectionV2(cfg, dbUser, dbPass, cfg.Host, dbPort); err != nil {
		return fmt.Errorf("SQL Server connection failed: %v", err)
	}

	// Execute backup
	script := fmt.Sprintf(`
if (-Not (Test-Path 'C:\temp')) { New-Item -ItemType Directory -Path 'C:\temp' -Force | Out-Null }
try {
    Invoke-Sqlcmd -ServerInstance '%s,%s' -Username '%s' -Password '%s' -Query "BACKUP DATABASE [%s] TO DISK = N'%s'" -QueryTimeout 600
    Write-Output 'BACKUP_OK'
} catch {
    Write-Error "Backup failed: $_"
    exit 1
}
`, cfg.Host, dbPort, dbUser, dbPass, dbName, tempPath)

	output, err := RunRemotePowerShell(cfg, script)
	if err != nil {
		return fmt.Errorf("backup script failed: %v", err)
	}
	if !strings.Contains(output, "BACKUP_OK") {
		log.Printf("Backup output: %s", output)
	}

	// Verify
	if err := verifyRemoteFileV2(cfg, tempPath); err != nil {
		return fmt.Errorf("backup file not found: %v", err)
	}

	// Copy to local
	if err := copyRemoteFileToLocalV2(cfg, tempPath, backupFile); err != nil {
		return fmt.Errorf("failed to copy backup: %v", err)
	}

	// Cleanup
	deleteRemoteFileV2(cfg, tempPath)
	return nil
}

func validateSQLServerConnectionV2(cfg RemoteExecConfig, dbUser, dbPass, dbHost, dbPort string) error {
	script := fmt.Sprintf(`Invoke-Sqlcmd -ServerInstance '%s,%s' -Username '%s' -Password '%s' -Query "SELECT 1" -QueryTimeout 30`, dbHost, dbPort, dbUser, dbPass)
	_, err := RunRemotePowerShell(cfg, script)
	return err
}

func verifyRemoteFileV2(cfg RemoteExecConfig, remotePath string) error {
	script := fmt.Sprintf(`if (Test-Path '%s') { Write-Output 'EXISTS' } else { Write-Output 'NOTFOUND'; exit 1 }`, remotePath)
	output, err := RunRemotePowerShell(cfg, script)
	if err != nil || !strings.Contains(output, "EXISTS") {
		return fmt.Errorf("file not found: %s", remotePath)
	}
	return nil
}

func copyRemoteFileToLocalV2(cfg RemoteExecConfig, remotePath, localPath string) error {
	log.Printf("Copying %s -> %s via %s", remotePath, localPath, cfg.ConnectionType)

	switch cfg.ConnectionType {
	case "winrm":
		// WinRM: read file as base64, decode locally
		script := fmt.Sprintf(`[Convert]::ToBase64String([IO.File]::ReadAllBytes('%s'))`, remotePath)
		b64, err := RunRemotePowerShell(cfg, script)
		if err != nil {
			return fmt.Errorf("WinRM file read failed: %v", err)
		}
		decodeCmd := fmt.Sprintf(`echo '%s' | base64 -d > '%s'`, strings.TrimSpace(b64), localPath)
		if output, err := exec.Command("bash", "-c", decodeCmd).CombinedOutput(); err != nil {
			return fmt.Errorf("base64 decode failed: %v (%s)", err, string(output))
		}
	default:
		scpPath := strings.ReplaceAll(remotePath, `\`, "/")
		cmd := exec.Command("sshpass", "-p", cfg.Password, "scp", "-o", "StrictHostKeyChecking=no",
			fmt.Sprintf("%s@%s:%s", cfg.User, cfg.Host, scpPath), localPath)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("SCP failed: %v (%s)", err, string(output))
		}
	}
	log.Printf("File copied to: %s", localPath)
	return nil
}

func deleteRemoteFileV2(cfg RemoteExecConfig, remotePath string) {
	script := fmt.Sprintf(`Remove-Item -Path '%s' -Force -ErrorAction SilentlyContinue`, remotePath)
	if _, err := RunRemotePowerShell(cfg, script); err != nil {
		log.Printf("Warning: could not delete %s: %v", remotePath, err)
	}
}

func fetchAndDeleteRemoteLogV2(cfg RemoteExecConfig, remotePath string) {
	script := fmt.Sprintf(`if (Test-Path '%s') { Get-Content '%s'; Remove-Item -Path '%s' -Force }`, remotePath, remotePath, remotePath)
	output, _ := RunRemotePowerShell(cfg, script)
	if output != "" {
		log.Printf("Remote log:\n%s", output)
	}
}

// ============== Legacy wrappers for existing callers ==============

func backupSQLServerWindows(dbHost, dbUser, dbPass, dbName, dbPort, backupDir, serverUser, serverPassword string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	return backupSQLServerWindowsV2(cfg, dbUser, dbPass, dbName, dbPort, backupDir)
}

func validateSQLServerConnection(dbHost, dbPort, dbUser, dbPass, serverUser, serverPassword string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	return validateSQLServerConnectionV2(cfg, dbUser, dbPass, dbHost, dbPort)
}

func executePowerShellScript(dbHost, dbUser, dbPass, serverUser, serverPassword, script string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	_, err := RunRemotePowerShell(cfg, script)
	return err
}

func verifyBackupFile(dbHost, dbUser, dbPass, serverUser, serverPassword, backupPath string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	return verifyRemoteFileV2(cfg, backupPath)
}

func copyBackupFile(dbHost, dbUser, dbPass, serverUser, serverPassword, tempBackupPath, backupFilePath string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	if err := copyRemoteFileToLocalV2(cfg, tempBackupPath, backupFilePath); err != nil {
		return err
	}
	deleteRemoteFileV2(cfg, tempBackupPath)
	return nil
}

func deleteBackupFile(dbHost, serverUser, serverPassword, tempBackupPath string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	deleteRemoteFileV2(cfg, tempBackupPath)
	return nil
}

func fetchAndLogBackupFile(dbHost, dbUser, dbPass, serverUser, serverPassword string) {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	fetchAndDeleteRemoteLogV2(cfg, `C:\temp\backup_log.txt`)
}

func deleteRemoteFile(dbHost, serverUser, serverPassword, filePath string) error {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	deleteRemoteFileV2(cfg, filePath)
	return nil
}

func detectSQLServerOS(dbHost, serverUser, serverPassword string) (string, error) {
	cfg := RemoteExecConfig{Host: dbHost, User: serverUser, Password: serverPassword, ConnectionType: "ssh"}
	return DetectSQLServerOSByConfig(cfg)
}
