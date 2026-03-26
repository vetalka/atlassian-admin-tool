package handlers

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func DropDatabase(dbKind, dbHost, dbName, dbUser, dbPass, dbPort, dbTmp, remoteUser, ServerPassword string) error {
	switch dbKind {
	case "org.postgresql.Driver":
		// Set the PGPASSWORD environment variable
		envVars := map[string]string{"PGPASSWORD": dbPass}

		// Dynamically find the full path to psql
		psqlPath, err := exec.LookPath("psql")
		if err != nil {
			return fmt.Errorf("psql not found: %v", err)
		}

		dropDatabase := fmt.Sprintf(`%s -h %s -d %s -U %s -p %s -tAc "DROP DATABASE %s;"`, psqlPath, dbHost, dbName, dbUser, dbPort, dbTmp)
		log.Printf("Executing command to drop temp PostgreSQL database: %s", dropDatabase)
		if err := runCommand(dropDatabase, envVars); err != nil {
			return fmt.Errorf("failed to drop database: %v", err)
		}

	case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
		// Detect whether SQL Server is on Linux or Windows
		sqlServerOS, err := detectSQLServerOS(dbHost, remoteUser, ServerPassword)
		if err != nil {
			return fmt.Errorf("failed to detect SQL Server OS: %v", err)
		}

		if sqlServerOS == "linux" {
			// Linux SQL Server Drop Database logic
			dropDatabaseCmd := fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P "%s" -Q "DROP DATABASE [%s];"`, dbHost, dbPort, dbUser, dbPass, dbTmp)
			sshCmd := fmt.Sprintf(`sshpass -p "%s" ssh %s@%s "%s"`, ServerPassword, remoteUser, dbHost, dropDatabaseCmd)

			log.Printf("Executing command to drop temp SQL Server database on Linux: %s", sshCmd)
			if err := runCommand(sshCmd, nil); err != nil {
				return fmt.Errorf("failed to drop SQL Server database on Linux: %v", err)
			}
		} else {
			// Escape password and prepare the Windows drop database command
			escapedDbPass := strings.ReplaceAll(dbPass, `"`, `\"`)
			dropDatabaseCmd := fmt.Sprintf(`Invoke-Sqlcmd -ServerInstance \"%s,%s\" -Username \"%s\" -Password \"%s\" -Query \"DROP DATABASE [%s];\"`,
				dbHost, dbPort, dbUser, escapedDbPass, dbTmp)

			powershellDropScript := fmt.Sprintf(`powershell.exe -Command "%s"`, dropDatabaseCmd)
			sshCmd := fmt.Sprintf(`sshpass -p "%s" ssh %s@%s '%s'`, ServerPassword, remoteUser, dbHost, powershellDropScript)
			log.Printf("Executing command to drop temp SQL Server database on Windows: %s", sshCmd)

			if err := runCommand(sshCmd, nil); err != nil {
				return fmt.Errorf("failed to drop SQL Server database on Windows: %v", err)
			}

			// Step: Delete the backup file from the Windows server
			deleteBackupCmd := fmt.Sprintf(`sshpass -p "%s" ssh %s@%s powershell.exe -Command "Remove-Item -Path 'C:\\temp\\%s.bak' -Force"`,
				ServerPassword, remoteUser, dbHost, dbTmp)

			log.Printf("Executing command to delete backup file: %s", deleteBackupCmd)
			if err := runCommand(deleteBackupCmd, nil); err != nil {
				log.Printf("Failed to delete backup file %s.bak from Windows server: %v", dbTmp, err)
				return fmt.Errorf("failed to delete backup file from Windows server: %v", err)
			}
			log.Printf("Backup file %s.bak deleted from Windows server.", dbTmp)
		}

	case "mysql":
		log.Println("MySQL drop database is not yet implemented.")
		return fmt.Errorf("unsupported database type: %s", dbKind)

	case "oracle":
		log.Println("Oracle drop database is not yet implemented.")
		return fmt.Errorf("unsupported database type: %s", dbKind)

	default:
		log.Printf("Unsupported database type: %s", dbKind)
		return fmt.Errorf("unsupported database type: %s", dbKind)
	}

	return nil
}

func HandleRestore(dbKind, dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, dbFilePath, tempRestoreFolder, remoteUser, ServerPassword, appType, eazybiDbName string) error {
	var err error

	// Based on the database type, detect and update the dbFilePath
	switch dbKind {
	case "org.postgresql.Driver":
		// PostgreSQL specific file detection
		dbFilePath, err = detectBackupFile(dbFilePath, dbName, ".sql", eazybiDbName)
		if err != nil {
			return fmt.Errorf("PostgreSQL: failed to detect backup file: %v", err)
		}

		// PostgreSQL restore logic for Confluence and Jira
		err = restorePostgres(dbKind, dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, dbFilePath, tempRestoreFolder, remoteUser, ServerPassword, appType)
		if err != nil {
			log.Printf("%s restore failed: %v", appType, err)
			return fmt.Errorf("%s restore failed: %v", appType, err)
		}

	case "mysql":
		// MySQL specific file detection (if implemented in the future)
		log.Println("MySQL restoration is not yet implemented.")
		return fmt.Errorf("unsupported database type: %s", dbKind)

	case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
		// SQL Server specific file detection
		dbFilePath, err = detectBackupFile(dbFilePath, dbName, ".bak", eazybiDbName)
		if err != nil {
			return fmt.Errorf("SQL Server: failed to detect backup file: %v", err)
		}

		// Detect the SQL Server OS (Linux or Windows)
		sqlServerOS, err := detectSQLServerOS(dbHost, remoteUser, ServerPassword)
		if err != nil {
			return fmt.Errorf("failed to detect SQL Server OS: %v", err)
		}

		// SQL Server restore logic for both Linux and Windows
		var RestoredVersion, CurrentVersion string
		if sqlServerOS == "linux" {
			RestoredVersion = "/tmp/restored_version.txt"
			CurrentVersion = "/tmp/current_version.txt"
			err = restoreSQLServerOnLinux(dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, dbFilePath, tempRestoreFolder, remoteUser, ServerPassword, appType)
		} else {
			RestoredVersion = "C:\\temp\\restored_version.txt"
			CurrentVersion = "C:\\temp\\current_version.txt"
			err = restoreSQLServerOnWindows(dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, dbFilePath, tempRestoreFolder, remoteUser, ServerPassword, appType)
		}

		if err != nil {
			log.Printf("%s restore failed: %v", appType, err)
			return fmt.Errorf("%s restore failed: %v", appType, err)
		}

		// Fetch and compare versions for both Confluence and Jira
		err = fetchAndCompareSQLServerVersions(dbKind, dbHost, dbUser, dbPass, dbName, dbTmp, dbPort, RestoredVersion, CurrentVersion, remoteUser, appType, ServerPassword)
		if err != nil {
			log.Printf("%s version mismatch detected: %v", appType, err)
			return fmt.Errorf("%s version mismatch detected: %v", appType, err)
		}

	case "oracle":
		// Oracle specific file detection (if implemented in the future)
		log.Println("Oracle restoration is not yet implemented.")
		return fmt.Errorf("unsupported database type: %s", dbKind)

	default:
		log.Printf("Unsupported database type: %s", dbKind)
		return fmt.Errorf("unsupported database type: %s", dbKind)
	}

	return nil
}

// detectBackupFile detects the appropriate backup file based on the database type,
// ensuring that the file name does not match the eazybiDbName.
func detectBackupFile(dbFilePath, dbName, extension, eazybiDbName string) (string, error) {
	// Check if dbFilePath is a directory
	fileInfo, err := os.Stat(dbFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to access backup file path: %v", err)
	}

	if fileInfo.IsDir() {
		// If the path is a directory, search for the correct file with the given extension
		backupFilePath, err := findBackupFile(dbFilePath, dbName, extension, eazybiDbName)
		if err != nil {
			return "", fmt.Errorf("no %s file found in the backup directory: %v", extension, err)
		}
		log.Printf("Found %s file for restoration: %s", extension, backupFilePath)
		return backupFilePath, nil
	}

	log.Printf("Backup file path is already a file: %s", dbFilePath)
	return dbFilePath, nil
}

func DetectMDFLDFFiles(dbName, dbHost, dbPort, dbUser, dbPass string) (string, string, string, string, error) {
	query := fmt.Sprintf(`
        SELECT mf.physical_name AS PhysicalFileName 
        FROM sys.master_files mf 
        JOIN sys.databases db ON mf.database_id = db.database_id 
        WHERE db.name = '%s' 
        ORDER BY mf.type_desc;`, dbName)

	// Log the SQL query for debugging purposes
	log.Printf("Executing SQL query: %s", query)

	// Execute SQL command to get MDF and LDF file paths
	cmd := exec.Command("sqlcmd", "-S", fmt.Sprintf("%s,%s", dbHost, dbPort), "-U", dbUser, "-P", dbPass, "-Q", query, "-h", "-1", "-W")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	// Log the sqlcmd command being executed for debugging purposes
	log.Printf("Executing sqlcmd: %s", strings.Join(cmd.Args, " "))

	if err := cmd.Run(); err != nil {
		// Log the stderr output in case of error
		log.Printf("Error output: %s", stderr.String())
		return "", "", "", "", fmt.Errorf("failed to execute SQL command: %v", err)
	}

	// Log the stdout output for debugging purposes
	log.Printf("Command stdout output: %s", out.String())

	// Parse the query result to extract MDF and LDF file paths
	var mdfFile, ldfFile string
	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".mdf") {
			mdfFile = line
		} else if strings.HasSuffix(line, ".ldf") {
			ldfFile = line
		}
	}

	// Determine the directory paths for MDF and LDF files
	var mdfDirPath, ldfDirPath string

	// Determine MDF directory path
	if strings.Contains(mdfFile, ":") {
		// Windows-style path
		mdfDirPath = strings.ReplaceAll(mdfFile[:strings.LastIndex(mdfFile, "\\")], "/", "\\")
	} else {
		// Unix-style path
		mdfDirPath = mdfFile[:strings.LastIndex(mdfFile, "/")]
		log.Printf("MDF Directory Path (Unix): '%s'", mdfDirPath) // Debugging
	}

	// Determine LDF directory path
	if strings.Contains(ldfFile, ":") {
		// Windows-style path
		ldfDirPath = strings.ReplaceAll(ldfFile[:strings.LastIndex(ldfFile, "\\")], "/", "\\")
	} else {
		// Unix-style path
		ldfDirPath = ldfFile[:strings.LastIndex(ldfFile, "/")]
		log.Printf("LDF Directory Path (Unix): '%s'", ldfDirPath) // Debugging
	}

	// Use MDF and LDF paths
	log.Printf("Detected MDF file: %s, LDF file: %s", mdfFile, ldfFile)

	return mdfFile, ldfFile, mdfDirPath, ldfDirPath, nil
}

// restorePostgres handles the restoration process for PostgreSQL.
func restorePostgres(dbKind, dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, dbFilePath, tempRestoreFolder, remoteUser, ServerPassword, appType string) error {
	// Set the PGPASSWORD environment variable
	envVars := map[string]string{"PGPASSWORD": dbPass}

	// Dynamically find the full path to psql
	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql not found: %v", err)
	}

	// Check if dbFilePath is a directory
	fileInfo, err := os.Stat(dbFilePath)
	if err != nil {
		return fmt.Errorf("failed to access dbFilePath: %v", err)
	}

	if fileInfo.IsDir() {
		// If it's a directory, construct the expected backup file name
		expectedFileName := fmt.Sprintf("%s.sql", dbName)
		expectedFilePath := filepath.Join(dbFilePath, expectedFileName)

		// Check if the expected file exists
		if _, err := os.Stat(expectedFilePath); err != nil {
			return fmt.Errorf("expected SQL file not found in the specified directory: %s", expectedFilePath)
		}

		// Update dbFilePath to the expected SQL file
		dbFilePath = expectedFilePath
		log.Printf("Using SQL file for restoration: %s", dbFilePath)
	}

	// Create a temporary database by connecting to the "postgres" database
	sshCmd := fmt.Sprintf(`%s -h %s -p %s -U %s -d postgres -c "CREATE DATABASE %s WITH ENCODING 'UNICODE' LC_COLLATE 'C' LC_CTYPE 'C' TEMPLATE template0;"`, psqlPath, dbHost, dbPort, dbUser, dbTmp)
	if err := runCommand(sshCmd, envVars); err != nil {
		return fmt.Errorf("failed to create temporary database: %v", err)
	}

	// Restore the database dump
	restoreCmd := fmt.Sprintf(`%s -h %s -U %s -p %s -d %s -f %s`, psqlPath, dbHost, dbUser, dbPort, dbTmp, dbFilePath)
	if err := runCommand(restoreCmd, envVars); err != nil {
		return fmt.Errorf("failed to restore database dump: %v", err)
	}

	// Alter the database and grant privileges
	alterCmd := fmt.Sprintf(`%s -h %s -U %s -p %s -d postgres -c "ALTER DATABASE %s OWNER TO %s;"`, psqlPath, dbHost, dbUser, dbPort, dbTmp, dbUser)
	if err := runCommand(alterCmd, envVars); err != nil {
		return fmt.Errorf("failed to alter database owner: %v", err)
	}

	grantCmd := fmt.Sprintf(`%s -h %s -U %s -p %s -d postgres -c "GRANT ALL PRIVILEGES ON DATABASE %s TO %s;"`, psqlPath, dbHost, dbUser, dbPort, dbTmp, dbUser)
	if err := runCommand(grantCmd, envVars); err != nil {
		return fmt.Errorf("failed to grant privileges on database: %v", err)
	}

	// Fetch versions and compare
	return fetchAndComparePostgresVersions(dbKind, dbHost, dbUser, dbPass, dbName, dbTmp, dbPort, tempRestoreFolder, appType, remoteUser, ServerPassword)
}

// fetchAndComparePostgresVersions fetches and compares the versions for PostgreSQL, based on appType (Jira or Confluence).
func fetchAndComparePostgresVersions(dbKind, dbHost, dbUser, dbPass, dbName, dbTmp, dbPort, tempRestoreFolder, appType, remoteUser, ServerPassword string) error {

	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql not found: %v", err)
	}

	// Ensure the directory exists
	if err := os.MkdirAll(tempRestoreFolder, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	log.Printf("Directory created or exists: %s", tempRestoreFolder)

	envVars := map[string]string{"PGPASSWORD": dbPass}

	// Determine the version query based on appType
	var currentVersionCmd, restoredVersionCmd string
	if appType == "Jira" {
		// Jira version query
		currentVersionCmd = fmt.Sprintf(`%s -h %s -d %s -U %s -p %s  -tAc "SELECT propertyvalue FROM propertystring WHERE id = (SELECT id FROM propertyentry WHERE property_key = 'jira.version.patched');" > %s/current_version.txt`, psqlPath, dbHost, dbName, dbUser, dbPort, tempRestoreFolder)
		restoredVersionCmd = fmt.Sprintf(`%s -h %s -d %s -U %s -p %s  -tAc "SELECT propertyvalue FROM propertystring WHERE id = (SELECT id FROM propertyentry WHERE property_key = 'jira.version.patched');" > %s/restored_version.txt`, psqlPath, dbHost, dbTmp, dbUser, dbPort, tempRestoreFolder)
	} else if appType == "Confluence" {
		// Confluence version query
		currentVersionCmd = fmt.Sprintf(`%s -h %s -d %s -U %s -p %s -tAc "SELECT BANDANAVALUE FROM BANDANA WHERE BANDANAKEY = 'version.history';" | grep -oP '(?<=<string>)[^<]+' | head -1 > %s/current_version.txt`, psqlPath, dbHost, dbName, dbUser, dbPort, tempRestoreFolder)
		restoredVersionCmd = fmt.Sprintf(`%s -h %s -d %s -U %s -p %s -tAc "SELECT BANDANAVALUE FROM BANDANA WHERE BANDANAKEY = 'version.history';" | grep -oP '(?<=<string>)[^<]+' | head -1 > %s/restored_version.txt`, psqlPath, dbHost, dbTmp, dbUser, dbPort, tempRestoreFolder)
	} else {
		return fmt.Errorf("unsupported appType: %s", appType)
	}

	// Execute version fetch commands
	log.Printf("Fetching current version for %s", appType)
	if err := runCommand(currentVersionCmd, envVars); err != nil {
		return fmt.Errorf("failed to fetch current version for %s: %v", appType, err)
	}

	log.Printf("Fetching restored version for %s", appType)
	if err := runCommand(restoredVersionCmd, envVars); err != nil {
		return fmt.Errorf("failed to fetch restored version for %s: %v", appType, err)
	}

	// Compare versions
	return compareVersionsPostgres(tempRestoreFolder, appType, dbKind, dbHost, dbTmp, dbUser, dbPass, dbPort, remoteUser, ServerPassword)
}

// restoreSQLServerOnLinux handles the restoration process for SQL Server on a Linux environment.
func restoreSQLServerOnLinux(dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, dbFilePath, tempRestoreFolder, remoteUser, ServerPassword, appType string) error {
	// Detect MDF and LDF files
	mdfFile, ldfFile, mdfDirPath, ldfDirPath, err := DetectMDFLDFFiles(dbName, dbHost, dbPort, dbUser, dbPass)
	if err != nil {
		return fmt.Errorf("failed to detect MDF and LDF files: %v", err)
	}

	// Use MDF and LDF paths
	log.Printf("Detected MDF file: %s, LDF file: %s", mdfFile, ldfFile)

	// Prepare paths for restoration
	tempSQLBackupPath := fmt.Sprintf("/tmp/%s", dbFilePath)
	mdfRestorePath := fmt.Sprintf("%s/%s.mdf", mdfDirPath, dbTmp)
	ldfRestorePath := fmt.Sprintf("%s/%s_log.ldf", ldfDirPath, dbTmp)

	// Copy backup file to SQL Server
	scpCmd := fmt.Sprintf(`sshpass -p "%s" scp %s %s@%s:%s`, ServerPassword, dbFilePath, remoteUser, dbHost, tempSQLBackupPath)
	if err := runCommand(scpCmd, nil); err != nil {
		return fmt.Errorf("failed to copy backup file to SQL Server: %v", err)
	}

	// Restore database from backup
	restoreCmd := fmt.Sprintf(`sshpass -p "%s" ssh %s@%s "sqlcmd -S %s,%s -U %s -P %s -Q \"RESTORE DATABASE [%s] FROM DISK = N'%s' WITH MOVE '%s' TO '%s', MOVE '%s_log' TO '%s'\""`, ServerPassword, remoteUser, dbHost, dbHost, dbPort, dbUser, dbPass, dbTmp, tempSQLBackupPath, dbName, mdfRestorePath, dbName, ldfRestorePath)
	if err := runCommand(restoreCmd, nil); err != nil {
		return fmt.Errorf("failed to restore SQL Server database: %v", err)
	}

	return nil
}

func restoreSQLServerOnWindows(dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, dbFilePath, tempRestoreFolder, remoteUser, ServerPassword, appType string) error {
	// Step 1: Ensure dbFilePath is a valid file
	fileInfo, err := os.Stat(dbFilePath)
	if err != nil {
		return fmt.Errorf("failed to access backup file: %v", err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("%s is a directory, not a regular file", dbFilePath)
	}

	// Step 2: Copy the .bak file to the Windows server (C:\temp)
	scpCmd := fmt.Sprintf(`sshpass -p "%s" scp %s %s@%s:/temp/%s.bak`, ServerPassword, dbFilePath, remoteUser, dbHost, dbTmp)
	if err := runCommand(scpCmd, nil); err != nil {
		return fmt.Errorf("failed to copy backup file to SQL Server: %v", err)
	}

	log.Printf("Backup file %s copied to Windows server successfully.", dbFilePath)

	// Step 3: Detect MDF and LDF file paths from the existing database
	mdfFile, ldfFile, mdfDirPath, ldfDirPath, err := DetectMDFLDFFiles(dbName, dbHost, dbPort, dbUser, dbPass)
	if err != nil {
		return fmt.Errorf("failed to detect MDF and LDF files: %v", err)
	}
	log.Printf("Detected MDF file: %s, LDF file: %s", mdfFile, ldfFile)

	// Step 4: Construct MDF and LDF restore paths using dbTmp
	mdfRestorePath := fmt.Sprintf("%s\\%s.mdf", mdfDirPath, dbTmp)
	ldfRestorePath := fmt.Sprintf("%s\\%s_log.ldf", ldfDirPath, dbTmp)

	// Step 5: Detect logical names using RESTORE FILELISTONLY from the backup file
	dataLogicalName, logLogicalName, err := DetectLogicalNamesFromBackup(remoteUser, ServerPassword, dbTmp, dbHost, dbPort, dbUser, dbPass)
	if err != nil {
		return fmt.Errorf("failed to detect logical names from backup: %v", err)
	}
	log.Printf("Detected logical data name: %s, logical log name: %s", dataLogicalName, logLogicalName)

	// Step 6: PowerShell script to restore the database on Windows
	powershellRestoreScript := fmt.Sprintf(`
    $SqlBackupPath = 'C:\\temp\\%s.bak';
    try {
        Invoke-Sqlcmd -ServerInstance '%s,%s' -Username '%s' -Password '%s' -QueryTimeout 600 -Query "
        RESTORE DATABASE [%s] FROM DISK = N'$SqlBackupPath'
        WITH MOVE '%s' TO '%s',
        MOVE '%s' TO '%s', REPLACE";
        # Verify the restoration
        if ($?) {
            Write-Host 'Database restoration completed successfully.';
        } else {
            Write-Host 'Database restoration failed.';
            exit 1;
        }
    } catch {
        Write-Host 'Database restoration failed: $_';
        exit 1;
    }
    `,
		dbTmp, dbHost, dbPort, dbUser, dbPass, dbTmp, dataLogicalName, escapeBackslashes(mdfRestorePath), logLogicalName, escapeBackslashes(ldfRestorePath))

	// Log the PowerShell command
	log.Printf("Executing PowerShell command: %s", powershellRestoreScript)

	// Execute the PowerShell script to restore the database
	if err := executePowerShellScript(dbHost, dbUser, dbPass, remoteUser, ServerPassword, powershellRestoreScript); err != nil {
		return fmt.Errorf("failed to execute PowerShell script for database restore: %v", err)
	}

	log.Printf("%s database restore command executed successfully.", appType)

	log.Printf("Database %s was successfully created and restored on SQL Server.", dbTmp)
	return nil
}

// Helper function to escape backslashes for PowerShell
func escapeBackslashes(path string) string {
	return strings.ReplaceAll(path, "\\", "\\\\")
}

// Helper function to escape quotes for PowerShell commands
func escapePowerShellQuotes(command string) string {
	return strings.ReplaceAll(command, `"`, `\"`)
}

func DetectLogicalNamesFromBackup(remoteUser, ServerPassword, dbTmp, dbHost, dbPort, dbUser, dbPass string) (string, string, error) {
	// Construct the RESTORE FILELISTONLY query
	query := fmt.Sprintf("RESTORE FILELISTONLY FROM DISK = N'C:\\temp\\%s.bak'", dbTmp)

	// Execute the SQL command using Invoke-Sqlcmd
	cmd := exec.Command("sshpass", "-p", ServerPassword, "ssh", fmt.Sprintf("%s@%s", remoteUser, dbHost),
		fmt.Sprintf(`powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '%s,%s' -Username '%s' -Password '%s' -Query \"%s\" "`,
			dbHost, dbPort, dbUser, dbPass, query))

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		log.Printf("Error executing SQL command: %s", stderr.String())
		log.Printf("Command output: %s", out.String())
		return "", "", fmt.Errorf("failed to execute SQL command: %v", err)
	}

	lines := strings.Split(out.String(), "\n")
	var dataLogicalName, logLogicalName string
	for _, line := range lines {
		// Find the lines containing 'LogicalName' and extract the value
		if strings.Contains(line, "LogicalName") {
			fields := strings.Fields(line)
			if len(fields) == 3 && fields[0] == "LogicalName" {
				if dataLogicalName == "" {
					dataLogicalName = fields[2] // The MDF logical name (first occurrence)
				} else {
					logLogicalName = fields[2] // The LDF logical name (second occurrence)
				}
			}
		}
	}

	// Ensure MDF and LDF logical names are found
	if dataLogicalName == "" || logLogicalName == "" {
		return "", "", fmt.Errorf("failed to detect logical names")
	}

	return dataLogicalName, logLogicalName, nil
}

func fetchAndCompareSQLServerVersions(dbKind, dbHost, dbUser, dbPass, dbName, dbTmp, dbPort, RestoredVersion, CurrentVersion, remoteUser, appType, ServerPassword string) error {
	// Prepare SQL command for current version
	var currentVersionCmd, restoredVersionCmd string

	if appType == "Jira" {
		// Jira version query
		currentVersionCmd = fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -d %s -Q "SELECT propertyvalue FROM propertystring WHERE id = (SELECT id FROM propertyentry WHERE property_key = 'jira.version.patched');" -h -1 -W`, dbHost, dbPort, dbUser, dbPass, dbName)
		restoredVersionCmd = fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -d %s -Q "SELECT propertyvalue FROM propertystring WHERE id = (SELECT id FROM propertyentry WHERE property_key = 'jira.version.patched');" -h -1 -W`, dbHost, dbPort, dbUser, dbPass, dbTmp)
	} else if appType == "Confluence" {
		// Confluence version query
		currentVersionCmd = fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -d %s -Q "SELECT BANDANAVALUE FROM BANDANA WHERE BANDANAKEY = 'version.history';" -h -1 -W`, dbHost, dbPort, dbUser, dbPass, dbName)
		restoredVersionCmd = fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -d %s -Q "SELECT BANDANAVALUE FROM BANDANA WHERE BANDANAKEY = 'version.history';" -h -1 -W`, dbHost, dbPort, dbUser, dbPass, dbTmp)
	} else {
		return fmt.Errorf("unsupported appType: %s", appType)
	}

	// Execute and capture current version
	log.Printf("Fetching current version for %s", appType)
	currentVersionOutput, err := runCommandCaptureOutput(currentVersionCmd)
	if err != nil {
		return fmt.Errorf("failed to fetch current version for %s: %v", appType, err)
	}

	// Write current version to file
	err = os.WriteFile(CurrentVersion, []byte(currentVersionOutput), 0644)
	if err != nil {
		return fmt.Errorf("failed to write current version to file: %v", err)
	}

	// Execute and capture restored version
	log.Printf("Fetching restored version for %s", appType)
	restoredVersionOutput, err := runCommandCaptureOutput(restoredVersionCmd)
	if err != nil {
		return fmt.Errorf("failed to fetch restored version for %s: %v", appType, err)
	}

	// Write restored version to file
	err = os.WriteFile(RestoredVersion, []byte(restoredVersionOutput), 0644)
	if err != nil {
		return fmt.Errorf("failed to write restored version to file: %v", err)
	}

	// Compare versions
	return compareVersionsSQL(RestoredVersion, CurrentVersion, appType, dbKind, dbHost, dbTmp, dbUser, dbPass, dbPort, remoteUser, ServerPassword)
}

func compareVersionsSQL(RestoredVersion, CurrentVersion, appType, dbKind, dbHost, dbTmp, dbUser, dbPass, dbPort, remoteUser, ServerPassword string) error {
	// Read the current version
	mainAppVersion, err := os.ReadFile(fmt.Sprintf("%s", CurrentVersion))
	if err != nil {
		return fmt.Errorf("failed to read current version: %v", err)
	}

	// Read the restored version
	restoredAppVersion, err := os.ReadFile(fmt.Sprintf("%s", RestoredVersion))
	if err != nil {
		return fmt.Errorf("failed to read restored version: %v", err)
	}

	// Compare the versions
	if strings.TrimSpace(string(mainAppVersion)) != strings.TrimSpace(string(restoredAppVersion)) {
		log.Printf("Version mismatch detected: Current %s version: %s, Restored %s version: %s", appType, mainAppVersion, appType, restoredAppVersion)

		// Drop the restored SQL Server database due to version mismatch
		if err := DropDatabase(dbKind, dbHost, dbTmp, dbUser, dbPass, dbPort, dbTmp, remoteUser, ServerPassword); err != nil {
			log.Printf("Failed to drop restored database: %v", err)
			return fmt.Errorf("version mismatch detected and failed to drop database: %v", err)
		}

		// Clean up temp version files
		err = os.Remove(CurrentVersion)
		if err != nil {
			log.Printf("Failed to delete current version file: %v", err)
		}
		err = os.Remove(RestoredVersion)
		if err != nil {
			log.Printf("Failed to delete restored version file: %v", err)
		}

		return fmt.Errorf("version mismatch detected for %s and restored database dropped", appType)
	}

	log.Printf("Versions are the same for %s, continuing with the restoration process.", appType)

	// Clean up temp version files
	err = os.Remove(CurrentVersion)
	if err != nil {
		log.Printf("Failed to delete current version file: %v", err)
	}
	err = os.Remove(RestoredVersion)
	if err != nil {
		log.Printf("Failed to delete restored version file: %v", err)
	}

	return nil
}

func compareVersionsPostgres(tempRestoreFolder, appType, dbKind, dbHost, dbTmp, dbUser, dbPass, dbPort, remoteUser, ServerPassword string) error {
	currentVersionPath := fmt.Sprintf("%s/current_version.txt", tempRestoreFolder)
	restoredVersionPath := fmt.Sprintf("%s/restored_version.txt", tempRestoreFolder)

	// Read the current version
	mainAppVersion, err := os.ReadFile(currentVersionPath)
	if err != nil {
		return fmt.Errorf("failed to read current version: %v", err)
	}

	// Read the restored version
	restoredAppVersion, err := os.ReadFile(restoredVersionPath)
	if err != nil {
		return fmt.Errorf("failed to read restored version: %v", err)
	}

	// Compare the versions
	if strings.TrimSpace(string(mainAppVersion)) != strings.TrimSpace(string(restoredAppVersion)) {
		log.Printf("Version mismatch detected: Current %s version: %s, Restored %s version: %s", appType, mainAppVersion, appType, restoredAppVersion)

		// Drop the restored PostgreSQL database due to version mismatch
		if err := DropDatabase(dbKind, dbHost, dbTmp, dbUser, dbPass, dbPort, dbTmp, remoteUser, ServerPassword); err != nil {
			log.Printf("Failed to drop restored database: %v", err)
			return fmt.Errorf("version mismatch detected and failed to drop database: %v", err)
		}

		// Clean up temp version files
		err = os.Remove(currentVersionPath)
		if err != nil {
			log.Printf("Failed to delete current version file: %v", err)
		}
		err = os.Remove(restoredVersionPath)
		if err != nil {
			log.Printf("Failed to delete restored version file: %v", err)
		}

		return fmt.Errorf("version mismatch detected for %s and restored database dropped", appType)
	}

	log.Printf("Versions are the same for %s, continuing with the restoration process.", appType)

	// Clean up temp version files
	err = os.Remove(currentVersionPath)
	if err != nil {
		log.Printf("Failed to delete current version file: %v", err)
	}
	err = os.Remove(restoredVersionPath)
	if err != nil {
		log.Printf("Failed to delete restored version file: %v", err)
	}

	return nil
}

// runCommand runs a shell command with optional environment variables and logs its output.
func runCommand(cmd string, envVars map[string]string) error {
	log.Printf("Executing command: %s", cmd)

	// Check if this is an sshpass command that might need WinRM redirection
	if strings.HasPrefix(strings.TrimSpace(cmd), "sshpass") {
		var winrmOut bytes.Buffer
		redirected, err := tryWinRMRedirect(cmd, &winrmOut)
		if winrmOut.Len() > 0 {
			log.Printf("Command output: %s", winrmOut.String())
		}
		if redirected {
			return err
		}
	}

	command := exec.Command("sh", "-c", cmd)

	// Set environment variables if provided
	if envVars != nil {
		for key, value := range envVars {
			command.Env = append(command.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}

	var out bytes.Buffer
	command.Stdout = &out
	command.Stderr = &out
	if err := command.Run(); err != nil {
		log.Printf("Command output: %s", out.String())
		return fmt.Errorf("command failed: %v", err)
	}
	log.Printf("Command output: %s", out.String())
	return nil
}

func runCommandCaptureOutput(cmd string) (string, error) {
	log.Printf("Executing command: %s", cmd)
	command := exec.Command("sh", "-c", cmd)

	var out bytes.Buffer
	command.Stdout = &out
	command.Stderr = &out
	if err := command.Run(); err != nil {
		log.Printf("Command output: %s", out.String())
		return "", fmt.Errorf("command failed: %v", err)
	}
	log.Printf("Command output: %s", out.String())
	return out.String(), nil
}

// findBackupFile searches for a backup file with a specific extension in the provided directory
// and matches the filename with the expected dbName, ensuring it doesn't match the eazybiDbName.
func findBackupFile(dirPath, dbName, ext string, eazybiDbName string) (string, error) {
	log.Printf("Searching for backup file in directory: %s", dirPath)

	files, err := os.ReadDir(dirPath)
	if err != nil {
		return "", fmt.Errorf("failed to read directory: %v", err)
	}

	log.Println("Files found in directory:")
	for _, file := range files {
		log.Printf("Checking file: '%s' (Expected: '%s' + '%s')", file.Name(), dbName, ext)

		// Normalize file name (trim spaces, lowercase)
		fileName := strings.TrimSpace(file.Name())
		expectedFile := strings.TrimSpace(dbName) + ext

		// Debugging output
		log.Printf("Comparing '%s' with expected '%s'", fileName, expectedFile)

		if fileName == expectedFile {
			log.Printf("Match found! Using backup file: %s", fileName)
			return filepath.Join(dirPath, fileName), nil
		}
	}

	return "", fmt.Errorf("no backup file matching database name '%s' with extension '%s' found in directory", dbName, ext)
}
