package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
	"golang.org/x/crypto/ssh"
)

// ─── Data structures ──────────────────────────────────────────────────────────

// BackupPolicy represents a scheduled backup policy.
type BackupPolicy struct {
	ID                int64
	Name              string
	EnvironmentID     int64
	EnvironmentName   string
	Schedule          string
	BackupTypes       []string
	DestinationFolder string
	RetentionDays     int
	Enabled           bool
	CreatedAt         string
	UpdatedAt         string
}

// BackupPolicyRun represents a single execution of a policy.
type BackupPolicyRun struct {
	ID              int64
	PolicyID        int64
	PolicyName      string
	StartedAt       string
	FinishedAt      string
	Status          string
	Log             string
	BackupSizeBytes int64
	FilesCreated    []string
	DiskUsage       string
}

// envSnapshot holds a point-in-time copy of environment fields needed for backup.
type envSnapshot struct {
	Name           string
	App            string
	IP             string
	ServerUser     string
	ServerPassword string
	HomeDir        string
	InstallDir     string
	SharedHomeDir  string
	DBName         string
	DBUser         string
	DBPass         string
	DBPort         string
	DBHost         string
	DBDriver       string
	ConnectionType string
	EazyBIDBName   string
	EazyBIDBUser   string
	EazyBIDBPass   string
	EazyBIDBPort   string
	EazyBIDBHost   string
}

// ─── DB helpers ───────────────────────────────────────────────────────────────

func loadBackupPolicy(policyID int64) (*BackupPolicy, error) {
	var p BackupPolicy
	var typesJSON string
	var enabled int
	err := db.QueryRow(`
		SELECT bp.id, bp.name, bp.environment_id, COALESCE(e.name,''),
		       bp.schedule, bp.backup_types, bp.destination_folder,
		       bp.retention_days, bp.enabled,
		       bp.created_at, bp.updated_at
		FROM backup_policies bp
		LEFT JOIN environments e ON e.id = bp.environment_id
		WHERE bp.id = ?`, policyID).
		Scan(&p.ID, &p.Name, &p.EnvironmentID, &p.EnvironmentName,
			&p.Schedule, &typesJSON, &p.DestinationFolder,
			&p.RetentionDays, &enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled == 1
	if err := json.Unmarshal([]byte(typesJSON), &p.BackupTypes); err != nil {
		p.BackupTypes = []string{}
	}
	return &p, nil
}

func loadEnvSnapshot(envID int64) (*envSnapshot, error) {
	var e envSnapshot
	err := db.QueryRow(`
		SELECT name, app, ip, server_user, server_password,
		       home_dir, install_dir, sharedhome_dir,
		       app_dbname, app_dbuser, app_dbpass, app_dbport, app_dbhost,
		       db_driver, db_connection_type,
		       COALESCE(eazybi_dbname,''), COALESCE(eazybi_dbuser,''),
		       COALESCE(eazybi_dbpass,''), COALESCE(eazybi_dbport,''),
		       COALESCE(eazybi_dbhost,'')
		FROM environments WHERE id = ?`, envID).
		Scan(&e.Name, &e.App, &e.IP, &e.ServerUser, &e.ServerPassword,
			&e.HomeDir, &e.InstallDir, &e.SharedHomeDir,
			&e.DBName, &e.DBUser, &e.DBPass, &e.DBPort, &e.DBHost,
			&e.DBDriver, &e.ConnectionType,
			&e.EazyBIDBName, &e.EazyBIDBUser, &e.EazyBIDBPass,
			&e.EazyBIDBPort, &e.EazyBIDBHost)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func createPolicyRun(policyID int64) (int64, error) {
	res, err := db.Exec(
		`INSERT INTO backup_policy_runs (policy_id, started_at, status, log)
		 VALUES (?, CURRENT_TIMESTAMP, 'running', '')`, policyID)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func appendRunLog(runID int64, msg string) {
	line := fmt.Sprintf("[%s] %s\n", time.Now().Format("15:04:05"), msg)
	db.Exec("UPDATE backup_policy_runs SET log = log || ? WHERE id = ?", line, runID)
	log.Printf("Run %d: %s", runID, msg)
}

func finishPolicyRun(runID int64, status string) {
	db.Exec(
		`UPDATE backup_policy_runs SET finished_at = CURRENT_TIMESTAMP, status = ? WHERE id = ?`,
		status, runID)
}

// ─── Main orchestrator ────────────────────────────────────────────────────────

// RunPolicy executes a backup policy by ID. Safe to call in a goroutine.
func RunPolicy(policyID int64) {
	runID, err := createPolicyRun(policyID)
	if err != nil {
		log.Printf("RunPolicy %d: cannot create run record: %v", policyID, err)
		return
	}

	appendRunLog(runID, fmt.Sprintf("Policy %d started.", policyID))

	policy, err := loadBackupPolicy(policyID)
	if err != nil {
		appendRunLog(runID, fmt.Sprintf("Cannot load policy: %v", err))
		finishPolicyRun(runID, "failed")
		return
	}

	env, err := loadEnvSnapshot(policy.EnvironmentID)
	if err != nil {
		appendRunLog(runID, fmt.Sprintf("Cannot load environment (id=%d): %v", policy.EnvironmentID, err))
		finishPolicyRun(runID, "failed")
		return
	}

	appendRunLog(runID, fmt.Sprintf("Policy %q | env=%s (%s) | types=%s",
		policy.Name, env.Name, env.App, strings.Join(policy.BackupTypes, ",")))

	// Create timestamped run directory
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	runDir := filepath.Join(policy.DestinationFolder, timestamp)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		appendRunLog(runID, fmt.Sprintf("Cannot create run dir %q: %v", runDir, err))
		finishPolicyRun(runID, "failed")
		return
	}
	appendRunLog(runID, fmt.Sprintf("Run directory: %s", runDir))

	// Expand "full" to all concrete types
	types := policy.BackupTypes
	if contains(types, "full") {
		types = []string{"database", "attachments", "nfs", "appdata"}
		if env.EazyBIDBName != "" {
			types = append(types, "eazybi")
		}
	}

	var filesCreated []string
	errCount := 0

	for _, bt := range types {
		destDir := filepath.Join(runDir, bt)
		if mkErr := os.MkdirAll(destDir, 0755); mkErr != nil {
			appendRunLog(runID, fmt.Sprintf("[%s] cannot create sub-dir: %v", bt, mkErr))
			errCount++
			continue
		}

		appendRunLog(runID, fmt.Sprintf("[%s] starting...", bt))

		var files []string
		var runErr error

		switch bt {
		case "database":
			files, runErr = runScheduledDatabaseBackup(env, destDir, runID)
		case "attachments":
			files, runErr = runScheduledAttachmentsBackup(env, destDir, runID)
		case "nfs":
			files, runErr = runScheduledNFSBackup(env, destDir, runID)
		case "appdata":
			files, runErr = runScheduledAppDataBackup(env, destDir, runID)
		case "eazybi":
			files, runErr = runScheduledEazyBIBackup(env, destDir, runID)
		default:
			appendRunLog(runID, fmt.Sprintf("[%s] unknown type — skipped", bt))
			continue
		}

		if runErr != nil {
			appendRunLog(runID, fmt.Sprintf("[%s] FAILED: %v", bt, runErr))
			errCount++
		} else {
			filesCreated = append(filesCreated, files...)
			appendRunLog(runID, fmt.Sprintf("[%s] done (%d file(s))", bt, len(files)))
		}
	}

	// Write manifest
	if mPath, mErr := writeRunManifest(runDir, policy, env, filesCreated); mErr == nil {
		filesCreated = append(filesCreated, mPath)
	} else {
		appendRunLog(runID, fmt.Sprintf("Manifest write error: %v", mErr))
	}

	// Persist file list and total size
	totalSize := calculateDirSize(runDir)
	filesJSON, _ := json.Marshal(filesCreated)
	db.Exec(`UPDATE backup_policy_runs SET files_created = ?, backup_size_bytes = ? WHERE id = ?`,
		string(filesJSON), totalSize, runID)

	// Apply retention
	applyRetention(policy, runID)

	// Determine final status
	finalStatus := "success"
	switch {
	case errCount > 0 && len(filesCreated) <= 1: // only manifest (or nothing)
		finalStatus = "failed"
	case errCount > 0:
		finalStatus = "partial"
	}

	appendRunLog(runID, fmt.Sprintf("Done. status=%s | size=%s | files=%d",
		finalStatus, humanize.Bytes(uint64(totalSize)), len(filesCreated)))
	finishPolicyRun(runID, finalStatus)

	// Bump policy updated_at
	db.Exec("UPDATE backup_policies SET updated_at = CURRENT_TIMESTAMP WHERE id = ?", policyID)
}

// ─── Per-type backup implementations ─────────────────────────────────────────

func runScheduledDatabaseBackup(env *envSnapshot, destDir string, runID int64) ([]string, error) {
	if env.DBHost == "" || env.DBName == "" {
		return nil, fmt.Errorf("database host/name not configured for environment %q", env.Name)
	}
	appendRunLog(runID, fmt.Sprintf("[database] host=%s db=%s driver=%s", env.DBHost, env.DBName, env.DBDriver))

	// Reuse existing BackupDatabaseForEnv — it handles pg_dump / sqlcmd / mssql
	if err := BackupDatabaseForEnv(env.Name, env.DBHost, env.DBUser, env.DBPass,
		env.DBName, env.DBPort, env.DBDriver, destDir); err != nil {
		return nil, err
	}

	// Collect whatever files BackupDatabaseForEnv wrote into destDir
	entries, _ := filepath.Glob(filepath.Join(destDir, "*"))
	return entries, nil
}

func runScheduledAttachmentsBackup(env *envSnapshot, destDir string, runID int64) ([]string, error) {
	client, err := sshClientForEnv(env)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	appendRunLog(runID, fmt.Sprintf("[attachments] home=%s shared=%s", env.HomeDir, env.SharedHomeDir))

	archivePath := filepath.Join(destDir, "Attachments.tar.gz")
	if err := runSSHTarToFile(client, buildAttachmentsCmd(env), archivePath); err != nil {
		return nil, err
	}
	return []string{archivePath}, nil
}

func runScheduledNFSBackup(env *envSnapshot, destDir string, runID int64) ([]string, error) {
	if env.SharedHomeDir == "" {
		return nil, fmt.Errorf("shared home not configured for environment %q", env.Name)
	}
	client, err := sshClientForEnv(env)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	appendRunLog(runID, fmt.Sprintf("[nfs] shared=%s", env.SharedHomeDir))

	archivePath := filepath.Join(destDir, "NFS.tar.gz")
	if err := runSSHTarToFile(client, buildNFSCmd(env), archivePath); err != nil {
		return nil, err
	}
	return []string{archivePath}, nil
}

func runScheduledAppDataBackup(env *envSnapshot, destDir string, runID int64) ([]string, error) {
	if env.HomeDir == "" {
		return nil, fmt.Errorf("home dir not configured for environment %q", env.Name)
	}
	client, err := sshClientForEnv(env)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	appendRunLog(runID, fmt.Sprintf("[appdata] home=%s", env.HomeDir))

	// Reuse BackupHomeDirectory which already handles Jira/Confluence/Bitbucket differences
	if err := BackupHomeDirectory(client, env.HomeDir, destDir, env.App); err != nil {
		return nil, err
	}

	entries, _ := filepath.Glob(filepath.Join(destDir, "*"))
	return entries, nil
}

// ─── SSH helpers ──────────────────────────────────────────────────────────────

func sshClientForEnv(env *envSnapshot) (*ssh.Client, error) {
	ips := strings.Split(env.IP, " ")
	ip := strings.TrimSpace(ips[0])
	client, err := connectToServer(ip, env.ServerUser, env.ServerPassword)
	if err != nil {
		return nil, fmt.Errorf("SSH connect to %s failed: %v", ip, err)
	}
	return client, nil
}

// runSSHTarToFile runs a remote tar command and streams stdout directly to a local file.
// This avoids buffering the entire archive in memory.
func runSSHTarToFile(client *ssh.Client, cmd, localPath string) error {
	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("SSH session error: %v", err)
	}
	defer sess.Close()

	outFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("cannot create %s: %v", localPath, err)
	}
	defer outFile.Close()

	var stderr bytes.Buffer
	sess.Stdout = outFile
	sess.Stderr = &stderr

	if err := sess.Run(cmd); err != nil {
		os.Remove(localPath)
		return fmt.Errorf("remote command failed: %v (stderr: %s)", err, stderr.String())
	}
	return nil
}

// buildAttachmentsCmd returns the appropriate tar command for the app type.
func buildAttachmentsCmd(env *envSnapshot) string {
	switch strings.ToLower(env.App) {
	case "jira":
		return fmt.Sprintf("sudo tar czf - -C %s/data .", env.HomeDir)
	case "confluence":
		return fmt.Sprintf("sudo tar czf - -C %s/attachments .", env.HomeDir)
	default:
		return fmt.Sprintf("sudo tar czf - -C %s .", env.HomeDir)
	}
}

// buildNFSCmd returns the tar command for the shared home (NFS) directory.
func buildNFSCmd(env *envSnapshot) string {
	src := env.SharedHomeDir
	switch strings.ToLower(env.App) {
	case "jira":
		return fmt.Sprintf(
			"sudo tar czf - -C %s --exclude='export' --exclude='import' --exclude='log' --exclude='data' .",
			src)
	case "confluence":
		return fmt.Sprintf(
			"sudo tar czf - -C %s --exclude='attachments' --exclude='backups' --exclude='logs' .",
			src)
	default:
		return fmt.Sprintf("sudo tar czf - -C %s .", src)
	}
}

// ─── Manifest ─────────────────────────────────────────────────────────────────

func writeRunManifest(runDir string, policy *BackupPolicy, env *envSnapshot, files []string) (string, error) {
	type manifest struct {
		PolicyName     string   `json:"policy_name"`
		PolicyID       int64    `json:"policy_id"`
		Environment    string   `json:"environment"`
		App            string   `json:"app"`
		BackupTypes    []string `json:"backup_types"`
		CreatedAt      string   `json:"created_at"`
		FilesCreated   []string `json:"files_created"`
		TotalSizeBytes int64    `json:"total_size_bytes"`
	}
	m := manifest{
		PolicyName:     policy.Name,
		PolicyID:       policy.ID,
		Environment:    env.Name,
		App:            env.App,
		BackupTypes:    policy.BackupTypes,
		CreatedAt:      time.Now().Format(time.RFC3339),
		FilesCreated:   files,
		TotalSizeBytes: calculateDirSize(runDir),
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(runDir, "manifest.json")
	return path, os.WriteFile(path, data, 0644)
}

// ─── Retention ────────────────────────────────────────────────────────────────

func applyRetention(policy *BackupPolicy, runID int64) {
	if policy.RetentionDays <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -policy.RetentionDays)

	entries, err := os.ReadDir(policy.DestinationFolder)
	if err != nil {
		appendRunLog(runID, fmt.Sprintf("Retention: cannot read %s: %v", policy.DestinationFolder, err))
		return
	}

	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Only touch directories that match our timestamp format
		t, err := time.Parse("2006-01-02_15-04-05", entry.Name())
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			dirPath := filepath.Join(policy.DestinationFolder, entry.Name())
			if rmErr := os.RemoveAll(dirPath); rmErr != nil {
				appendRunLog(runID, fmt.Sprintf("Retention: failed to remove %s: %v", dirPath, rmErr))
			} else {
				removed++
				log.Printf("Retention: removed old backup %s", dirPath)
			}
		}
	}
	if removed > 0 {
		appendRunLog(runID, fmt.Sprintf("Retention: removed %d old backup dir(s) (>%d days)", removed, policy.RetentionDays))
	}
}

// ─── Utilities ────────────────────────────────────────────────────────────────

// calculateDirSize returns the total byte size of all files under path.
func calculateDirSize(path string) int64 {
	var size int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// policyDiskUsage returns a human-readable size string for a policy's destination folder.
func policyDiskUsage(destFolder string) string {
	size := calculateDirSize(destFolder)
	if size == 0 {
		return "0 B"
	}
	return humanize.Bytes(uint64(size))
}

func runScheduledEazyBIBackup(env *envSnapshot, destDir string, runID int64) ([]string, error) {
	if env.EazyBIDBName == "" {
		return nil, fmt.Errorf("no EazyBI database configured for environment %q", env.Name)
	}
	appendRunLog(runID, fmt.Sprintf("[eazybi] host=%s db=%s driver=%s", env.EazyBIDBHost, env.EazyBIDBName, env.DBDriver))

	if err := BackupDatabaseForEnv(env.Name+"-eazybi", env.EazyBIDBHost, env.EazyBIDBUser, env.EazyBIDBPass,
		env.EazyBIDBName, env.EazyBIDBPort, env.DBDriver, destDir); err != nil {
		return nil, err
	}

	entries, _ := filepath.Glob(filepath.Join(destDir, "*"))
	return entries, nil
}
