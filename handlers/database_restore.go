package handlers

import (
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"bytes"
	"time"
	"os"
)

// buildCfgFromRestore creates a RemoteExecConfig for restore operations, 
// auto-detecting whether to use WinRM or SSH based on the environment config.
func buildCfgFromRestore(dbHost, remoteUser, serverPassword string) RemoteExecConfig {
	return BuildDBRemoteConfigFromHost(dbHost, remoteUser, serverPassword)
}


var (
    sqlcmdPath string
    bcpPath    string
)

// Table list for different applications
var appTableMap = map[string][]string{
    "Jira": {
        "propertyentry", "oauthspconsumer", "AO_21F425_MESSAGE_AO", "AO_FE1BC5_CLIENT",
        "AO_723324_CLIENT_CONFIG", "mailserver", "AO_2C4E5C_MAILITEMCHUNK", "AO_2C4E5C_MAILITEM",
        "AO_2C4E5C_MAILITEMAUDIT", "AO_54307E_EMAILCHANNELSETTING", "AO_2C4E5C_MAILCONNECTION",
        "AO_2C4E5C_MAILCHANNEL", "AO_2C4E5C_MAILHANDLER", "AO_ED669C_IDP_CONFIGserver", "AO_ED669C_SEEN_ASSERTIONS",
        "AO_93F03B_API_TOKEN_OBJECT", "AO_93F03B_RESTRICT_ENDPOINT",
    },
    "Confluence": {
        "bandana", "propertyentry", "oauthspconsumer", "AO_21F425_MESSAGE_AO", "AO_FE1BC5_CLIENT",
        "AO_723324_CLIENT_CONFIG", "mailserver", "AO_2C4E5C_MAILITEMCHUNK", "AO_2C4E5C_MAILITEM",
        "AO_2C4E5C_MAILITEMAUDIT", "AO_54307E_EMAILCHANNELSETTING", "AO_2C4E5C_MAILCONNECTION", "AO_2C4E5C_MAILCHANNEL",
        "AO_2C4E5C_MAILHANDLER", "AO_ED669C_IDP_CONFIGserver", "AO_ED669C_SEEN_ASSERTIONS", "AO_93F03B_API_TOKEN_OBJECT",
        "AO_93F03B_RESTRICT_ENDPOINT",
    },
	"eazyBI": {
        "system_settings",
    },
}

// RestoreDatabase handles the restoration process for the databases (Jira, Confluence).
func RestoreDatabase(envName, dbKind, dbHost, dbPort, dbUser, dbPass, dbName, dbTmp, dbTmp2, tempRestoreFolder, remoteUser, serverPassword, baseUrl, remoteTempFolder, app string) error {
    log.Printf("Restoring %s Database...", app)

    client, err := connectToServer(dbHost, remoteUser, serverPassword)
    if err != nil {
        return fmt.Errorf("failed to connect via SSH to %s: %v", dbHost, err)
    }
    defer client.Close()

    date, timeStr := getCurrentDateTime()

    switch dbKind {
    case "org.postgresql.Driver":
        // Handle PostgreSQL restoration
        if err := restorePostgresDatabase(dbHost, dbUser, dbPass, dbName, dbTmp, dbTmp2, remoteUser, serverPassword, baseUrl, tempRestoreFolder); err != nil {
			return fmt.Errorf("PostgreSQL %s database restore failed: %v", app, err)
        }
        if err := switchPostgresTables(app, dbHost, dbPort, dbUser, dbPass, dbTmp2, dbName, baseUrl, remoteTempFolder); err != nil {
            return fmt.Errorf("PostgreSQL table switching for %s failed: %v", app, err)
        }
    case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
        // Detect OS (Linux or Windows)
        osType, err := detectSQLServerOS(dbHost, remoteUser, serverPassword)
        if err != nil {
            return fmt.Errorf("failed to detect SQL Server OS: %v", err)
        }

        // Handle SQL Server restoration based on OS type
        if osType == "linux" {
            if err := restoreSQLServerOnLinuxDBSwitch(dbHost, dbPort, dbUser, dbPass, dbName, dbTmp, remoteUser, serverPassword); err != nil {
                return fmt.Errorf("SQL Server (Linux) %s database restore failed: %v", app, err)
            }
        } else if osType == "windows" {
			// Assuming you already have the correct values for dbFilePath, tempRestoreFolder, and appType:
			if err := restoreSQLServerOnWindowsDBSwitch(dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, tempRestoreFolder, remoteUser, serverPassword, app, date, timeStr); err != nil {
				return fmt.Errorf("SQL Server (Windows) %s database restore failed: %v", app, err)
			}
        }

        if err := switchSQLServerTables(app, dbHost, dbPort, dbUser, dbPass, dbName, remoteTempFolder, baseUrl, osType, serverPassword, remoteUser, date, timeStr); err != nil {
            return fmt.Errorf("SQL Server table switching for %s failed: %v", app, err)
        }
    case "mysql":
        return fmt.Errorf("MySQL restoration for %s is not yet implemented", app)
    case "oracle":
        return fmt.Errorf("Oracle restoration for %s is not yet implemented", app)
    default:
        return fmt.Errorf("unsupported database type: %s for %s", dbKind, app)
    }

    return nil
}

// RestoreEazyBI handles the restoration process for the eazyBI databases.
func RestoreEazyBI(envName, dbKind, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbTmpEazybi, tempRestoreFolder, remoteUser, serverPassword, remoteTempFolder, eazybiDbFilePath string) error {
    log.Printf("Restoring Eazybi Database...")

    client, err := connectToServer(eazybiDbHost, remoteUser, serverPassword)
    if err != nil {
        return fmt.Errorf("failed to connect via SSH to %s: %v", eazybiDbHost, err)
    }
    defer client.Close()

    switch dbKind {
    case "org.postgresql.Driver":
        // Handle PostgreSQL restoration
        if eazybiDbName != "" {
            if err := restorePostgresEazyBIDatabase(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, dbTmpEazybi, eazybiDbName, remoteUser, serverPassword, remoteTempFolder, tempRestoreFolder, eazybiDbFilePath); err != nil {
                return fmt.Errorf("PostgreSQL eazyBI database restore failed: %v", err)
            }
        }
    case "com.microsoft.sqlserver.jdbc.SQLServerDriver":
        // Detect OS (Linux or Windows)
        osType, err := detectSQLServerOS(eazybiDbHost, remoteUser, serverPassword)
        if err != nil {
            return fmt.Errorf("failed to detect SQL Server OS: %v", err)
        }

        // Handle SQL Server restoration based on OS type
        if osType == "linux" {
            if err := restoreSQLServerEazyBIOnLinux(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbTmpEazybi, tempRestoreFolder, eazybiDbFilePath, remoteUser, serverPassword); err != nil {
                return fmt.Errorf("SQL Server (Linux) eazyBI database restore failed: %v", err)
            }
        } else if osType == "windows" {
            if err := restoreSQLServerEazyBIOnWindows(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbTmpEazybi, tempRestoreFolder, eazybiDbFilePath, remoteUser, serverPassword); err != nil {
                return fmt.Errorf("SQL Server (Windows) eazyBI database restore failed: %v", err)
            }
        }
    case "mysql":
        return fmt.Errorf("MySQL restoration is not yet implemented")
    case "oracle":
        return fmt.Errorf("Oracle restoration is not yet implemented")
    default:
        return fmt.Errorf("unsupported database type: %s", dbKind)
    }

    return nil
}

// restorePostgresDatabase restores the Jira PostgreSQL database.
func restorePostgresDatabase(dbHost, dbUser, dbPass, dbName, dbTmp, dbTmp2, remoteUser, serverPassword, baseUrl, remoteTempFolder string) error {
	
	envVars := map[string]string{"PGPASSWORD": dbPass}

	// Dynamically find the full path to psql
	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql not found: %v", err)
	}

	commands := []string{
		fmt.Sprintf(`%s -U %s -h %s -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND datname = '%s';"`, psqlPath, dbUser, dbHost, dbName),
		fmt.Sprintf(`%s -U %s -h %s -d postgres -c "ALTER DATABASE %s RENAME TO %s;"`, psqlPath, dbUser, dbHost, dbName, dbTmp2),
		fmt.Sprintf(`%s -U %s -h %s -d postgres -c "ALTER DATABASE %s RENAME TO %s;"`, psqlPath, dbUser, dbHost, dbTmp, dbName),
		fmt.Sprintf(`%s -U %s -h %s -d postgres -c "ALTER DATABASE %s OWNER TO %s;"`, psqlPath, dbUser, dbHost, dbName, dbUser),
		fmt.Sprintf(`%s -U %s -h %s -d postgres -c "GRANT ALL PRIVILEGES ON DATABASE %s TO %s;"`, psqlPath, dbUser, dbHost, dbName, dbUser),
	}

	if err := executeCommandsWithPassword(commands, envVars); err != nil {
		return fmt.Errorf("failed to restore PostgreSQL database: %v", err)
	}

	log.Println("PostgreSQL Database restored successfully.")
	return nil
}

// restorePostgresEazyBIDatabase restores the eazyBI PostgreSQL database.
func restorePostgresEazyBIDatabase(eazybidbHost, eazybiDbPort, eazybidbUser, eazybidbPass, dbTmpEazybi, eazybidbName, remoteUser, serverPassword, remoteTempFolder, tempRestoreFolder, eazybiDbFilePath string) error {

	envVars := map[string]string{"PGPASSWORD": eazybidbPass}

	// Dynamically find the full path to psql
	psqlPath, err := exec.LookPath("psql")
	if err != nil {
		return fmt.Errorf("psql not found: %v", err)
	}

	// Dynamically find the full path to pg_dump
	pgdumpPath, err := exec.LookPath("pg_dump")
	if err != nil {
		return fmt.Errorf("pg_dump not found: %v", err)
	}

	commands := []string{
		// Terminate existing connections and rename the old database
		fmt.Sprintf(`%s -U %s -h %s -p %s -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE pid <> pg_backend_pid() AND datname = '%s';"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName),
		fmt.Sprintf(`%s -U %s -h %s -p %s -d postgres -c "ALTER DATABASE %s RENAME TO %s;"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName, dbTmpEazybi),
		fmt.Sprintf(`%s -U %s -h %s -p %s -d postgres -c "CREATE DATABASE %s WITH OWNER = %s ENCODING = 'UTF8' LC_COLLATE = 'en_US.UTF-8' LC_CTYPE = 'en_US.UTF-8';"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName, eazybidbUser),
		fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -f %s`,psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName, eazybiDbFilePath),
		fmt.Sprintf(`%s -U %s -h %s -p %s -d postgres -c "GRANT ALL PRIVILEGES ON DATABASE %s TO %s;"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName, eazybidbUser),
		fmt.Sprintf(`%s -U %s -h %s -p %s -d postgres -c "ALTER DATABASE %s OWNER TO %s;"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName, eazybidbUser),
	}

	if err := executeCommandsWithPassword(commands,envVars); err != nil {
		return fmt.Errorf("failed to restore PostgreSQL eazyBI database: %v", err)
	}

	// Eazy Advance Setting Restore
	checkTableCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -tAc "SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = 'system_settings');"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, dbTmpEazybi)
	output, err := runCommandRestoreOutput(checkTableCmd, envVars)
	if err != nil {
		return fmt.Errorf("failed to check for system_settings table: %v", err)
	}

	if strings.TrimSpace(output) == "t" {
		log.Println("system_settings table exists, proceeding with the restore.")

		dumpCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -t system_settings > /tmp/system_settings.sql`,pgdumpPath, eazybidbUser, eazybidbHost, eazybiDbPort, dbTmpEazybi)
		if err := runCommandRestore(dumpCmd, envVars); err != nil {
			return fmt.Errorf("failed to dump system_settings table: %v", err)
		}

		truncateCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -c "TRUNCATE TABLE system_settings;"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName)
		dropCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -c "DROP TABLE IF EXISTS system_settings;"`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName)
		restoreCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -f /tmp/system_settings.sql`, psqlPath, eazybidbUser, eazybidbHost, eazybiDbPort, eazybidbName)

		if err := executeCommandsWithPassword([]string{truncateCmd, dropCmd, restoreCmd}, envVars); err != nil {
			return fmt.Errorf("failed to restore system_settings table: %v", err)
		}

		log.Println("EazyBI system_settings table processed successfully.")
	} else {
		log.Println("system_settings table does not exist, skipping...")
	}

	log.Println("PostgreSQL eazyBI Database restored successfully.")
	
	// Clean up temporary files
	removeTempCmd := fmt.Sprintf("rm -rf /tmp/system_settings.sql")
	if err := runCommandRestore(removeTempCmd, nil); err != nil {
		log.Printf("Failed to remove temporary files: %v", err)
	}

	return nil
}

// switchPostgresTables handles table switching for PostgreSQL for multiple apps.
func switchPostgresTables(app, dbHost, dbPort, dbUser, dbPass, dbTmp2, dbName, baseUrl, remoteTempFolder string) error {
    envVars := map[string]string{"PGPASSWORD": dbPass}

    // Dynamically find the full path to psql
    psqlPath, err := exec.LookPath("psql")
    if err != nil {
        return fmt.Errorf("psql not found: %v", err)
    }

    // Fetch the table list for the app
    tables, exists := appTableMap[app]
    if !exists {
        return fmt.Errorf("no table list found for app: %s", app)
    }

    for _, table := range tables {
        checkTableCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -tAc "SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = '%s')"`, psqlPath, dbUser, dbHost, dbPort, dbTmp2, table)
        output, err := runCommandRestoreOutput(checkTableCmd, envVars)
        if err != nil || strings.TrimSpace(output) != "t" {
            log.Printf("Table %s does not exist or error occurred, skipping...", table)
            continue
        }

        dumpCmd := fmt.Sprintf(`pg_dump -U %s -h %s -p %s -d %s -t %s > /tmp/%s.sql`, dbUser, dbHost, dbPort, dbTmp2, table, table)
        if err := runCommandRestore(dumpCmd, envVars); err != nil {
            log.Printf("Failed to dump table %s: %v", table, err)
            continue
        }

        truncateCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -c "TRUNCATE TABLE %s;"`, psqlPath, dbUser, dbHost, dbPort, dbName, table)
        dropCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -c "DROP TABLE IF EXISTS %s;"`, psqlPath, dbUser, dbHost, dbPort, dbName, table)
        restoreCmd := fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -f /tmp/%s.sql`, psqlPath, dbUser, dbHost, dbPort, dbName, table)

        if err := executeCommandsWithPassword([]string{truncateCmd, dropCmd, restoreCmd}, envVars); err != nil {
            log.Printf("Failed to switch table %s: %v", table, err)
            continue
        }

        log.Printf("Table %s processed successfully.", table)

        // Clean up temporary files
        removeTempCmd := fmt.Sprintf("rm -rf /tmp/%s.sql", table)
        if err := runCommandRestore(removeTempCmd, nil); err != nil {
            log.Printf("Failed to remove temporary files: %v", err)
        }
    }

    // Base URL update logic for Jira and Confluence
    var baseUrlCmd string

    if app == "Jira" {
        // Jira specific base URL update
        baseUrlCmd = fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -c "UPDATE propertystring SET propertyvalue = '%s' FROM propertyentry PE WHERE PE.id = propertystring.id AND PE.property_key = 'jira.baseurl';"`, psqlPath, dbUser, dbHost, dbPort, dbName, baseUrl)
    } else if app == "Confluence" {
        // Confluence specific base URL update
        baseUrlCmd = fmt.Sprintf(`%s -U %s -h %s -p %s -d %s -c "UPDATE BANDANA SET BANDANAVALUE = REPLACE(BANDANAVALUE, '<baseUrl>.*</baseUrl>', '<baseUrl>%s</baseUrl>') WHERE BANDANACONTEXT = '_GLOBAL' AND BANDANAKEY = 'atlassian.confluence.settings';"`, psqlPath, dbUser, dbHost, dbPort, dbName, baseUrl)
    } else {
        return fmt.Errorf("unsupported application: %s", app)
    }

    // Execute the base URL update command
    if err := runCommandRestore(baseUrlCmd, envVars); err != nil {
        return fmt.Errorf("failed to update base URL for %s: %v", app, err)
    }

    log.Printf("Base URL for %s changed successfully to: %s", app, baseUrl)
    return nil
}

// restoreSQLServerOnLinuxDBSwitch restores the Jira SQL Server database.
func restoreSQLServerOnLinuxDBSwitch(dbHost, dbPort, dbUser, dbPass, dbName, dbTmp, remoteUser, serverPassword string) error {
	envVars := map[string]string{"MSSQL_PWD": dbPass}
	
	commands := []string{
		fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -Q "ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; ALTER DATABASE [%s] MODIFY NAME = %s_old; ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; ALTER DATABASE [%s] MODIFY NAME = %s; ALTER DATABASE [%s] SET MULTI_USER;"`, dbHost, dbPort, dbUser, dbPass, dbName, dbName, dbName, dbTmp, dbTmp, dbName, dbName),
	}

	if err := executeCommandsWithPassword(commands, envVars); err != nil {
		return fmt.Errorf("failed to restore SQL Server Jira database: %v", err)
	}

	log.Println("SQL Server Jira Database restored successfully.")
	return nil
}

func restoreSQLServerOnWindowsDBSwitch(dbHost, dbUser, dbPass, dbName, dbPort, dbTmp, tempRestoreFolder, remoteUser, serverPassword, appType, date, timeStr string) error {

	log.Printf("Detected dbName : %s, dbHost: %s, dbPort: %s, dbUser: %s, dbPass: %s.", dbName, dbHost, dbPort, dbUser, dbPass)

	envVars := map[string]string{"MSSQL_PWD": dbPass}

	// On Windows, we assume that sqlcmd and bcp are available in the system path
	tempRestoreFolder = `C:\\temp`

    // Step 1: Detect MDF and LDF file paths from the existing database
    mdfFile, ldfFile, mdfDirPath, ldfDirPath, err := DetectMDFLDFFiles(dbName, dbHost, dbPort, dbUser, dbPass)
    if err != nil {
        return fmt.Errorf("failed to detect MDF and LDF files: %v", err)
    }

    // Use MDF and LDF paths
    log.Printf("Detected MDF file: %s, LDF file: %s", mdfFile, ldfFile)

    dataLogicalName, logLogicalName, err := DetectLogicalNamesFromBackup(remoteUser, serverPassword, dbTmp, dbHost, dbPort, dbUser, dbPass)
    if err != nil {
        return fmt.Errorf("failed to detect logical names from backup: %v", err)
    }
    log.Printf("Detected logical data name: %s, logical log name: %s", dataLogicalName, logLogicalName)

    // Step 3: Set the original database to SINGLE_USER and rename the databases
    sqlCmd := fmt.Sprintf(`
        sqlcmd -S %s,%s -U %s -P %s -Q "
        ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
        ALTER DATABASE [%s] MODIFY NAME = [%s_%s_%s];
        ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
        ALTER DATABASE [%s] MODIFY NAME = [%s];
        ALTER DATABASE [%s] SET OFFLINE WITH ROLLBACK IMMEDIATE;
        ALTER DATABASE [%s_%s_%s] SET OFFLINE WITH ROLLBACK IMMEDIATE;"`,
        dbHost, dbPort, dbUser, dbPass, dbName, 
        dbName, dbName, date, timeStr, 
        dbTmp, dbTmp, dbName, dbName, 
        dbName, date, timeStr)

    // Execute the SQL commands for renaming and setting the databases offline
    if err := runCommand(sqlCmd, nil); err != nil {
        return fmt.Errorf("failed to execute SQL commands for database renaming: %v", err)
    }

	// Step 4: Rename MDF/LDF files using PowerShell
	powershellScript := fmt.Sprintf(`try { Rename-Item -Path '%s\%s.mdf' -NewName '%s\%s_%s_%s.mdf' -ErrorAction Stop; Rename-Item -Path '%s\%s_log.ldf' -NewName '%s\%s_%s_%s_log.ldf' -ErrorAction Stop; Rename-Item -Path '%s\%s.mdf' -NewName '%s\%s.mdf' -ErrorAction Stop; Rename-Item -Path '%s\%s_log.ldf' -NewName '%s\%s_log.ldf' -ErrorAction Stop; Write-Host 'Files renamed successfully.' } catch { Write-Host 'Error renaming files: ' $_; exit 1; }`, 
		mdfDirPath, dbName, mdfDirPath, dbName, date, timeStr,
		ldfDirPath, dbName, ldfDirPath, dbName, date, timeStr,
		mdfDirPath, dbTmp, mdfDirPath, dbName,
		ldfDirPath, dbTmp, ldfDirPath, dbName)

	// Escape the script properly to pass through ssh
	escapedPowerShellScript := strings.ReplaceAll(powershellScript, `"`, `\"`)

	// Execute PowerShell script remotely using SSH
	sshCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "%s"`, serverPassword, remoteUser, dbHost, escapedPowerShellScript)

	if err := runCommand(sshCmd, nil); err != nil {
		return fmt.Errorf("failed to rename MDF and LDF files via PowerShell: %v", err)
	}

    // Step 5: Modify MDF/LDF paths in SQL Server and bring the databases back online
    sqlRestoreCmd := fmt.Sprintf(`
    sqlcmd -S %s,%s -U %s -P %s -Q "
    ALTER DATABASE [%s_%s_%s] MODIFY FILE (NAME = '%s', FILENAME = '%s\%s_%s_%s.mdf');
    ALTER DATABASE [%s_%s_%s] MODIFY FILE (NAME = '%s', FILENAME = '%s\%s_%s_%s_log.ldf');
    ALTER DATABASE [%s] MODIFY FILE (NAME = '%s', FILENAME = '%s\%s.mdf');
    ALTER DATABASE [%s] MODIFY FILE (NAME = '%s', FILENAME = '%s\%s_log.ldf');
    ALTER DATABASE [%s] SET ONLINE;
    ALTER DATABASE [%s] SET MULTI_USER;
    ALTER DATABASE [%s_%s_%s] SET ONLINE;
    ALTER DATABASE [%s_%s_%s] SET MULTI_USER;"`,
        dbHost, dbPort, dbUser, dbPass,
        dbName, date, timeStr, dataLogicalName, mdfDirPath, dbName, date, timeStr,
        dbName, date, timeStr, logLogicalName, ldfDirPath, dbName, date, timeStr,
        dbName, dataLogicalName, mdfDirPath, dbName,
        dbName, logLogicalName, ldfDirPath, dbName,
        dbName, dbName, dbName, date, timeStr, dbName, date, timeStr)

    // Execute the SQL commands for modifying MDF/LDF paths and bringing the databases online
    if err := runCommand(sqlRestoreCmd, nil); err != nil {
        return fmt.Errorf("failed to modify MDF/LDF paths and bring databases online: %v", err)
    }

	// Step 6: Delete the restore file from the remote server after restoration
	remoteRestoreFile := fmt.Sprintf("%s\\%s.bak", tempRestoreFolder, dbTmp)
	deleteRestoreCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "del %s"`, serverPassword, remoteUser, dbHost, remoteRestoreFile)
	log.Printf("Deleting restore file on remote server: %s", remoteRestoreFile)

	// Execute the delete command to clean up the restore file
	log.Printf("Removing remote .bak file %s", deleteRestoreCmd)
	if err := runCommandRestore(deleteRestoreCmd, envVars); err != nil {
		log.Printf("Failed to remove remote %s.bak file: %v", dbTmp, err)
	} else {
		log.Printf("Successfully deleted restore file on remote server: %s", remoteRestoreFile)
	}

	log.Printf("Backup of current database %s stored under new database %s on SQL Server.", dbName, dbTmp)
    log.Printf("Database %s was successfully created and restored on SQL Server.", dbName)
    return nil
}

// restoreSQLServerEazyBIOnLinux restores the eazyBI SQL Server database.
func restoreSQLServerEazyBIOnLinux(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbTmpEazybi, tempRestoreFolder, eazybiDbFilePath, remoteUser, serverPassword string) error {
	envVars := map[string]string{"MSSQL_PWD": eazybiDbPass}

	commands := []string{
		fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -Q "ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; ALTER DATABASE [%s] MODIFY NAME = %s; RESTORE DATABASE [%s] FROM DISK = N'%s'; ALTER DATABASE [%s] SET MULTI_USER;"`, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, eazybiDbName, dbTmpEazybi, eazybiDbName, tempRestoreFolder, eazybiDbName),
	}

	if err := executeCommandsWithPassword(commands, envVars); err != nil {
		return fmt.Errorf("failed to restore SQL Server eazyBI database: %v", err)
	}

	log.Println("SQL Server eazyBI Database restored successfully.")
	return nil
}

/*
// restoreSQLServerEazyBIOnWindows restores the eazyBI SQL Server database.
func restoreSQLServerEazyBIOnWindows(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbTmpEazybi, tempRestoreFolder, eazybiDbFilePath, remoteUser, serverPassword string) error {
	envVars := map[string]string{"MSSQL_PWD": eazybiDbPass}

	commands := []string{
		fmt.Sprintf(`sqlcmd -S %s,%s -U %s -P %s -Q "ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE; ALTER DATABASE [%s] MODIFY NAME = %s_old; RESTORE DATABASE [%s] FROM DISK = N'%s'; ALTER DATABASE [%s] SET MULTI_USER;"`, eazybiDbHost, eazybiDbUser, eazybiDbPass, eazybiDbName, eazybiDbName, eazybiDbName, eazybiDbName, eazybiDbFilePath, eazybiDbName),
	}

	if err := executeCommandsWithPassword(commands, envVars); err != nil {
		return fmt.Errorf("failed to restore SQL Server eazyBI database: %v", err)
	}

	log.Println("SQL Server eazyBI Database restored successfully.")
	return nil
}
*/

// switchSQLServerTables handles the table switching for SQL Server.
func switchSQLServerTables(app, dbHost, dbPort, dbUser, dbPass, dbName, remoteTempFolder, baseUrl, osType, serverPassword, remoteUser, date, timeStr string) error {
	envVars := map[string]string{"MSSQL_PWD": dbPass}

	localTempFolder := "/tmp"

	// Set remoteTempFolder and detect sqlcmd/bcp paths based on the OS
	if osType == "linux" {
		remoteTempFolder = "/tmp"
		// Automatically detect the full path to sqlcmd
		sqlcmdPath, err := exec.LookPath("sqlcmd")
		if err != nil {
			return fmt.Errorf("sqlcmd not found: %v", err)
		}
		log.Printf("Detected sqlcmd path: %s", sqlcmdPath)

		// Automatically detect the full path to bcp
		bcpPath, err := exec.LookPath("bcp")
		if err != nil {
			return fmt.Errorf("bcp not found: %v", err)
		}
		log.Printf("Detected bcp path: %s", bcpPath)
	} else {
		// On Windows, we assume that sqlcmd and bcp are available in the system path
		remoteTempFolder = `C:\\temp`
		sqlcmdPath = `sqlcmd`
		bcpPath =`bcp`
	}

	// Automatically detect the full path to sqlcmd for local process 
	sqlcmdPathLocal, err := exec.LookPath("sqlcmd")
	if err != nil {
		return fmt.Errorf("sqlcmd not found: %v", err)
	}
	log.Printf("Detected sqlcmd path: %s", sqlcmdPathLocal)

	// Automatically detect the full path to bcp
	bcpPathLocal, err := exec.LookPath("bcp")
	if err != nil {
		return fmt.Errorf("bcp not found: %v", err)
	}
	log.Printf("Detected bcp path: %s", bcpPathLocal)

	// Dynamically select tables based on the application (app)
	tables, exists := appTableMap[app]
	if !exists {
		return fmt.Errorf("no table list found for app: %s", app)
	}

	for _, table := range tables {
		// Convert the table name to lowercase for case-insensitive comparison
		checkTableCmd := fmt.Sprintf(`%s -S %s,%s -U %s -P %s -d %s_%s_%s -Q "SET NOCOUNT ON; SELECT 1 WHERE OBJECT_ID('%s', 'U') IS NOT NULL;" -h -1 -W`, sqlcmdPathLocal, dbHost, dbPort, dbUser, dbPass, dbName, date, timeStr, table)

		// Log the command being executed for table checking
		log.Printf("Executing command to check table existence: %s", checkTableCmd)

		output, err := runCommandRestoreOutput(checkTableCmd, envVars)
		if err != nil || strings.TrimSpace(output) != "1" {
			log.Printf("Table %s does not exist or error occurred, skipping...", table)
			continue
		}

		// Export the table data and create a format file (.fmt) directly on the remote SQL Server
		var dumpCmd, fmtCmd string
		if osType == "windows" {
			// For Windows, escape backslashes in the commands
			dumpCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s_%s_%s.dbo.%s out %s\\%s.bcp -c -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbName, date, timeStr, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)

			// Create the .fmt file on the remote server
			fmtCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s_%s_%s.dbo.%s format nul -c -f %s\\%s.fmt -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbName, date, timeStr, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		} else {
			// For Linux, normal forward slashes
			dumpCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s_%s_%s.dbo.%s out %s/%s.bcp -c -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbName, date, timeStr, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)

			// Create the .fmt file on the remote server
			fmtCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s_%s_%s.dbo.%s format nul -c -f %s/%s.fmt -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbName, date, timeStr, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		}

		// Execute the dump and format commands directly on the remote server
		if err := runCommandRestore(dumpCmd, nil); err != nil {
			log.Printf("Failed to dump table %s directly on SQL Server: %v", table, err)
			continue
		}
		if err := runCommandRestore(fmtCmd, nil); err != nil {
			log.Printf("Failed to create format file for table %s directly on SQL Server: %v", table, err)
			continue
		}
		
		// Truncate the existing table on SQL Server
		truncateCmd := fmt.Sprintf(`%s -S %s,%s -U %s -P %s -d %s -Q "TRUNCATE TABLE %s;"`, sqlcmdPathLocal, dbHost, dbPort, dbUser, dbPass, dbName, table)
		if err := runCommandRestore(truncateCmd, envVars); err != nil {
			log.Printf("Failed to truncate table %s: %v", table, err)
			continue
		}
				
		// Perform the bulk insert using the .fmt file
		var bulkInsertCmd string
		if osType == "windows" {
			// For Windows, properly escape backslashes and row terminators
			bulkInsertCmd = fmt.Sprintf(
				`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "bcp %s.dbo.%s in %s\\\\%s.bcp -f %s\\\\%s.fmt -S %s,%s -U %s -P %s"`,
				serverPassword, remoteUser, dbHost, dbName, table, remoteTempFolder, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		} else {
			// For Linux, use standard forward slashes
			bulkInsertCmd = fmt.Sprintf(
				`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "bcp %s.dbo.%s in %s/%s.bcp -f %s/%s.fmt -S %s,%s -U %s -P %s"`,
				serverPassword, remoteUser, dbHost, dbName, table, remoteTempFolder, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		}

		if err := runCommandRestore(bulkInsertCmd, nil); err != nil {
			log.Printf("Failed to bulk insert table %s on SQL Server: %v", table, err)
			continue
		}
		// Step 1: Clean up the local .bcp file
		localBCPFile := fmt.Sprintf("%s/%s.bcp", localTempFolder, table)
		log.Printf("Removing local .bcp file: %s", localBCPFile)
		if err := os.Remove(localBCPFile); err != nil {
			log.Printf("Failed to remove local .bcp file for table %s: %v", table, err)
		} else {
			log.Printf("Successfully removed local .bcp file for table %s.", table)
		}
	 
		// Step 2: Clean up the remote .bcp .fmt files on the SQL Server
		var removeRemoteBCPCmd string
		var removeRemoteFMTCmd string

		if osType == "windows" {
			removeRemoteBCPCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "del %s\\%s.bcp"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)
			removeRemoteFMTCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "del %s\\%s.fmt"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)

		} else {
			removeRemoteBCPCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "rm -rf %s/%s.bcp"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)
			removeRemoteFMTCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "rm -rf %s/%s.fmt"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)

		}
	 
		log.Printf("Removing remote .bcp file for table %s: %s", table, removeRemoteBCPCmd)
		if err := runCommandRestore(removeRemoteBCPCmd, envVars); err != nil {
			log.Printf("Failed to remove remote .bcp or .fmt file for table %s: %v", table, err)
		} else {
			log.Printf("Successfully removed remote .bcp and .fmt file for table %s.", table)
		}

		log.Printf("Removing remote .fmt file for table %s: %s", table, removeRemoteFMTCmd)
		if err := runCommandRestore(removeRemoteBCPCmd, envVars); err != nil {
			log.Printf("Failed to remove remote .bcp or .fmt file for table %s: %v", table, err)
		} else {
			log.Printf("Successfully removed remote .bcp and .fmt file for table %s.", table)
		}

		log.Printf("Table %s processed successfully.", table)
	}

	// Base URL update logic for Jira and Confluence
	var baseUrlCmd string

	if app == "Jira" {
		baseUrlCmd = fmt.Sprintf(`%s -S %s,%s -U %s -P %s -d %s -Q "UPDATE propertystring SET propertyvalue = '%s' FROM propertyentry WHERE propertyentry.id = propertystring.id AND propertyentry.property_key = 'jira.baseurl';"`, sqlcmdPathLocal, dbHost, dbPort, dbUser, dbPass, dbName, baseUrl)
	} else if app == "Confluence" {
		baseUrlCmd = fmt.Sprintf(`%s -S %s,%s -U %s -P %s -d %s -Q "UPDATE BANDANA SET BANDANAVALUE = REPLACE(BANDANAVALUE, '<baseUrl>.*</baseUrl>', '<baseUrl>%s</baseUrl>') WHERE BANDANACONTEXT = '_GLOBAL' AND BANDANAKEY = 'atlassian.confluence.settings';"`, sqlcmdPathLocal, dbHost, dbPort, dbUser, dbPass, dbName, baseUrl)
	} else if app == "eazyBI" {
		// If eazyBI doesn't require a base URL update, skip this step or add relevant commands
		log.Println("No base URL update required for eazyBI.")
	} else {
		return fmt.Errorf("unsupported application: %s", app)
	}

	// Execute the base URL update command if applicable
	if baseUrlCmd != "" {
		if err := runCommandRestore(baseUrlCmd, envVars); err != nil {
			return fmt.Errorf("failed to update base URL for %s: %v", app, err)
		}
		log.Printf("Base URL for %s changed successfully to: %s", app, baseUrl)
	}

	return nil
}

// Generic function to execute a list of commands with environment variables
func executeCommandsWithPassword(commands []string, envVars map[string]string) error {
	for _, cmd := range commands {
		if err := runCommandRestore(cmd, envVars); err != nil {
			return err
		}
	}
	return nil
}

// Executes a command and returns its output
func runCommandRestoreOutput(cmd string, envVars map[string]string) (string, error) {
	var out bytes.Buffer
	if err := runCommandRestoreInternal(cmd, envVars, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

// Runs a command with environment variables and logs output
func runCommandRestore(cmd string, envVars map[string]string) error {
	var out bytes.Buffer
	if err := runCommandRestoreInternal(cmd, envVars, &out); err != nil {
		log.Printf("Command output: %s", out.String())
		return err
	}
	log.Printf("Command output: %s", out.String())
	return nil
}

// Internal function for running commands and managing output
// WinRM-aware: intercepts sshpass commands and redirects to WinRM when configured
func runCommandRestoreInternal(cmd string, envVars map[string]string, output *bytes.Buffer) error {
	log.Printf("Executing command: %s", cmd)

	// Check if this is an sshpass command that might need WinRM redirection
	if strings.HasPrefix(strings.TrimSpace(cmd), "sshpass") {
		redirected, err := tryWinRMRedirect(cmd, output)
		if redirected {
			return err
		}
		// If not redirected (SSH config), fall through to normal execution
	}

	command := exec.Command("sh", "-c", cmd)
	if envVars != nil {
		for key, value := range envVars {
			command.Env = append(command.Env, fmt.Sprintf("%s=%s", key, value))
		}
	}
	command.Stdout = output
	command.Stderr = output
	if err := command.Run(); err != nil {
		return fmt.Errorf("command failed: %v", err)
	}
	return nil
}

// tryWinRMRedirect parses an sshpass command string, checks if the target host
// is configured for WinRM, and executes via WinRM if so.
// Returns (true, err) if redirected to WinRM, (false, nil) if should use SSH.
func tryWinRMRedirect(cmd string, output *bytes.Buffer) (bool, error) {
	// Extract password: sshpass -p "PASSWORD" ...
	passRegex := regexp.MustCompile(`sshpass\s+-p\s+"([^"]*)"`)
	passMatch := passRegex.FindStringSubmatch(cmd)
	if passMatch == nil {
		return false, nil
	}
	password := passMatch[1]

	// Extract user@host
	userHostRegex := regexp.MustCompile(`(\S+)@(\S+)\s`)
	uhMatch := userHostRegex.FindStringSubmatch(cmd)
	if uhMatch == nil {
		return false, nil
	}
	user := uhMatch[1]
	host := uhMatch[2]

	// Look up if this host is configured for WinRM
	cfg := BuildDBRemoteConfigFromHost(host, user, password)
	if cfg.ConnectionType != "winrm" {
		return false, nil // Let the original SSH execution handle it
	}

	log.Printf("WinRM redirect: %s@%s via WinRM", cfg.User, cfg.Host)

	// Determine if it's a PowerShell command, SCP, or regular command
	if strings.Contains(cmd, "powershell.exe -Command") {
		// Extract PowerShell script
		psRegex := regexp.MustCompile(`powershell\.exe\s+-Command\s+"(.*)"`)
		psMatch := psRegex.FindStringSubmatch(cmd)
		if psMatch != nil {
			script := psMatch[1]
			// Unescape the script (remove SSH escaping)
			script = strings.ReplaceAll(script, `\\\"`, `"`)
			script = strings.ReplaceAll(script, `\'`, `'`)
			result, err := ExecRemotePowerShellCmd(cfg, script)
			output.Write(result)
			return true, err
		}
	} else if strings.Contains(cmd, "scp") {
		// SCP command — handle file copy via WinRM
		// Extract source and destination from scp command
		scpRegex := regexp.MustCompile(`scp\s+.*?"?(\S+)"?\s+(\S+)@(\S+):(\S+)`)
		scpMatch := scpRegex.FindStringSubmatch(cmd)
		if scpMatch != nil {
			localFile := strings.Trim(scpMatch[1], `"`)
			remotePath := scpMatch[4]
			err := CopyFileToRemote(cfg, localFile, remotePath)
			return true, err
		}
		// Reverse direction: scp user@host:remote local
		scpRevRegex := regexp.MustCompile(`scp\s+.*?(\S+)@(\S+):(\S+)\s+(\S+)`)
		scpRevMatch := scpRevRegex.FindStringSubmatch(cmd)
		if scpRevMatch != nil {
			remotePath := scpRevMatch[3]
			localFile := scpRevMatch[4]
			err := CopyFileFromRemote(cfg, remotePath, localFile)
			return true, err
		}
	} else {
		// Regular command (bcp, del, etc.)
		// Extract the command after user@host
		cmdRegex := regexp.MustCompile(`\S+@\S+\s+"?(.*?)"?\s*$`)
		cmdMatch := cmdRegex.FindStringSubmatch(cmd)
		if cmdMatch != nil {
			remoteCommand := strings.Trim(cmdMatch[1], `"`)
			result, err := ExecRemoteCommand(cfg, remoteCommand)
			output.Write(result)
			return true, err
		}
	}

	return false, nil // Couldn't parse, fall through to SSH
}

// Get current date and time for file renaming
func getCurrentDateTime() (string, string) {
    currentTime := time.Now()
    date := currentTime.Format("2006_01_02")
    timeStr := currentTime.Format("15_04_05")
    return date, timeStr
}





// restoreSQLServerEazyBIOnWindows restores the eazyBI SQL Server database.
func restoreSQLServerEazyBIOnWindows(eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbTmpEazybi, tempRestoreFolder, dbFilePath, remoteUser, serverPassword string) error {
	
	passwordEscaped := strings.ReplaceAll(eazybiDbPass, "!", "\\!")
	passwordEscaped = strings.ReplaceAll(passwordEscaped, "'", `'"'"'`)
	passwordEscaped = strings.ReplaceAll(passwordEscaped, `"`, `\"`)
	
	// On Windows, we assume that sqlcmd and bcp are available in the system path
	tempRestoreFolder = `C:\\temp`

	// Step 1: Ensure dbFilePath is a valid file
	fileInfo, err := os.Stat(dbFilePath)
	if err != nil {
		return fmt.Errorf("failed to access backup file: %v", err)
	}
	if fileInfo.IsDir() {
		return fmt.Errorf("%s is a directory, not a regular file", dbFilePath)
	}

	// Step 2: Detect MDF and LDF file paths from the existing database
	mdfFile, ldfFile, mdfDirPath, ldfDirPath, err := DetectMDFLDFFiles(eazybiDbName, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass)
	if err != nil {
		return fmt.Errorf("failed to detect MDF and LDF files: %v", err)
	}
	log.Printf("Detected MDF file: %s, LDF file: %s", mdfFile, ldfFile)

	// Step 3: Copy the .bak file to the Windows server (C:\temp)
	scpCmd := fmt.Sprintf(`sshpass -p "%s" scp -o StrictHostKeyChecking=no "%s" %s@%s:%s\\%s.bak`,
		serverPassword, dbFilePath, remoteUser, eazybiDbHost, tempRestoreFolder, dbTmpEazybi)
	if err := runCommandRestore(scpCmd, nil); err != nil {
		return fmt.Errorf("failed to copy backup file to SQL Server: %v", err)
	}
	log.Printf("Backup file %s copied to Windows server successfully.", dbFilePath)

	// Step 4: Detect logical names from the backup
	dataLogicalName, logLogicalName, err := DetectLogicalNamesFromBackup(remoteUser, serverPassword, dbTmpEazybi, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass)
	if err != nil {
		return fmt.Errorf("failed to detect logical names from backup: %v", err)
	}
	log.Printf("Detected logical data name: %s, logical log name: %s", dataLogicalName, logLogicalName)

	// Step 5: Construct SQL commands to terminate connections and rename the database
	terminateCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"ALTER DATABASE [%s] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName)

	renameCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"ALTER DATABASE [%s] MODIFY NAME = [%s];\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, eazybiDbName, dbTmpEazybi)

	setOfflineCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"ALTER DATABASE [%s] SET OFFLINE WITH ROLLBACK IMMEDIATE;\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, dbTmpEazybi)

	// Step 6: Execute termination and rename commands
	if err := runCommandRestore(terminateCmd, nil); err != nil {
		return fmt.Errorf("failed to terminate connections: %v", err)
	}
	if err := runCommandRestore(renameCmd, nil); err != nil {
		return fmt.Errorf("failed to rename database: %v", err)
	}
	if err := runCommandRestore(setOfflineCmd, nil); err != nil {
		return fmt.Errorf("failed to set database offline: %v", err)
	}

	// Step 7: Rename MDF/LDF files using PowerShell
	powershellScript := fmt.Sprintf(`try { Rename-Item -Path '%s\%s.mdf' -NewName '%s\%s.mdf' -ErrorAction Stop; Rename-Item -Path '%s\%s_log.ldf' -NewName '%s\%s_log.ldf' -ErrorAction Stop; Write-Host 'Files renamed successfully.' } catch { Write-Host 'Error renaming files: ' $_; exit 1; }`, 
		mdfDirPath, eazybiDbName, mdfDirPath, dbTmpEazybi,
		ldfDirPath, eazybiDbName, ldfDirPath, dbTmpEazybi)

	// Escape the script properly to pass through ssh
	escapedPowerShellScript := strings.ReplaceAll(powershellScript, `"`, `\"`)

	// Execute PowerShell script remotely using SSH
	sshCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "%s"`, serverPassword, remoteUser, eazybiDbHost, escapedPowerShellScript)

	if err := runCommand(sshCmd, nil); err != nil {
		return fmt.Errorf("failed to rename MDF and LDF files via PowerShell: %v", err)
	}

	// Step 8: Modify MDF and LDF file paths for the renamed database using the detected paths
	modifyFileCmd := fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"ALTER DATABASE [%s] MODIFY FILE (NAME = ''%s'', FILENAME = ''%s\\%s.mdf''); ALTER DATABASE [%s] MODIFY FILE (NAME = ''%s'', FILENAME = ''%s\\%s_log.ldf'');\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass,
		dbTmpEazybi, dataLogicalName, mdfDirPath, dbTmpEazybi, dbTmpEazybi, logLogicalName, ldfDirPath, dbTmpEazybi)

	log.Printf("Executing modify file paths command: %s", modifyFileCmd)
	if err := runCommandRestore(modifyFileCmd, nil); err != nil {
		return fmt.Errorf("failed to modify MDF and LDF file paths: %v", err)
	}

	// Step 9: Construct and execute the restore command
	restoreCmd := fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"RESTORE DATABASE [%s] FROM DISK = ''%s\\%s.bak'' WITH MOVE ''%s'' TO ''%s\\%s.mdf'', MOVE ''%s'' TO ''%s\\%s_log.ldf'', REPLACE;\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass,
		eazybiDbName, tempRestoreFolder, dbTmpEazybi, dataLogicalName, mdfDirPath, eazybiDbName,
		logLogicalName, ldfDirPath, eazybiDbName)

	log.Printf("Executing restore command: %s", restoreCmd)
	if err := runCommandRestore(restoreCmd, nil); err != nil {
		return fmt.Errorf("failed to restore database: %v", err)
	}

	// Pre step 10 : set backuped database online 
	setOnnlineCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"ALTER DATABASE [%s] SET ONLINE WITH ROLLBACK IMMEDIATE;\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, dbTmpEazybi)
	if err := runCommandRestore(setOnnlineCmd, nil); err != nil {
			return fmt.Errorf("failed to set database offline: %v", err)
	}
		
	// Step 10: Advanced eazyBI Settings Restore (system_settings table)
	err = switchSQLServerTablesEazybi(
		"eazyBI",				 // App name
		eazybiDbHost,            // dbHost
		eazybiDbPort,            // dbPort
		eazybiDbUser,            // dbUser
		eazybiDbPass,            // dbPass
		eazybiDbName,            // dbName
		dbTmpEazybi,			 // dbTmp
		tempRestoreFolder, 		 // remoteTempFolder
		"windows",               // osType
		serverPassword,          // serverPassword
		remoteUser,              // remoteUser
	)
	if err != nil {
		return fmt.Errorf("failed to switch tables for eazyBI: %v", err)
	}

	// Post step 10 : set backuped database offline 
	setOffTMPlineCmd := fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"ALTER DATABASE [%s] SET OFFLINE WITH ROLLBACK IMMEDIATE;\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass, dbTmpEazybi)
	if err := runCommandRestore(setOffTMPlineCmd, nil); err != nil {
			return fmt.Errorf("failed to set database offline: %v", err)
	}

	// Step 11: Grant permissions and set database ownership
	grantPermissionsCmd := fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Invoke-Sqlcmd -ServerInstance '\"%s,%s\"' -Username '\"%s\"' -Password '\"%s\"' -Query '\"ALTER DATABASE [%s] SET MULTI_USER; ALTER AUTHORIZATION ON DATABASE::[%s] TO [%s];\"'"`,
		serverPassword, remoteUser, eazybiDbHost, eazybiDbHost, eazybiDbPort, eazybiDbUser, eazybiDbPass,
		eazybiDbName, eazybiDbName, eazybiDbUser)

	log.Printf("Executing grant permissions command: %s", grantPermissionsCmd)
	if err := runCommandRestore(grantPermissionsCmd, nil); err != nil {
		return fmt.Errorf("failed to set permissions and ownership: %v", err)
	}

	// Step 12: Clean up eazyBI backup file from the Windows server
	log.Println("Cleaning up eazyBI backup file from C:\\temp...")

	cleanupBackupFileCmd := fmt.Sprintf(
		`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s powershell.exe -Command "Remove-Item -Path 'C:\\temp\\%s.bak' -Force"`,
		serverPassword, remoteUser, eazybiDbHost, dbTmpEazybi)

	if err := runCommandRestore(cleanupBackupFileCmd, nil); err != nil {
		log.Printf("Failed to remove eazyBI backup file from C:\\temp: %v", err)
	} else {
		log.Printf("eazyBI backup file %s.bak successfully removed from C:\\temp.", dbTmpEazybi)
	}

	log.Println("SQL Server eazyBI Database restored successfully.")

	return nil
}


// switchSQLServerTables handles the table switching for SQL Server.
func switchSQLServerTablesEazybi(app, dbHost, dbPort, dbUser, dbPass, dbName, dbTmpEazybi, remoteTempFolder, osType, serverPassword, remoteUser string) error {
	envVars := map[string]string{"MSSQL_PWD": dbPass}

	localTempFolder := "/tmp"

	// Set remoteTempFolder and detect sqlcmd/bcp paths based on the OS
	if osType == "linux" {
		remoteTempFolder = "/tmp"
		// Automatically detect the full path to sqlcmd
		sqlcmdPath, err := exec.LookPath("sqlcmd")
		if err != nil {
			return fmt.Errorf("sqlcmd not found: %v", err)
		}
		log.Printf("Detected sqlcmd path: %s", sqlcmdPath)

		// Automatically detect the full path to bcp
		bcpPath, err := exec.LookPath("bcp")
		if err != nil {
			return fmt.Errorf("bcp not found: %v", err)
		}
		log.Printf("Detected bcp path: %s", bcpPath)
	} else {
		// On Windows, we assume that sqlcmd and bcp are available in the system path
		remoteTempFolder = `C:\\temp`
		sqlcmdPath = `sqlcmd`
		bcpPath =`bcp`
	}

	// Automatically detect the full path to sqlcmd for local process 
	sqlcmdPathLocal, err := exec.LookPath("sqlcmd")
	if err != nil {
		return fmt.Errorf("sqlcmd not found: %v", err)
	}
	log.Printf("Detected sqlcmd path: %s", sqlcmdPathLocal)

	// Automatically detect the full path to bcp
	bcpPathLocal, err := exec.LookPath("bcp")
	if err != nil {
		return fmt.Errorf("bcp not found: %v", err)
	}
	log.Printf("Detected bcp path: %s", bcpPathLocal)

	// Dynamically select tables based on the application (app)
	tables, exists := appTableMap[app]
	if !exists {
		return fmt.Errorf("no table list found for app: %s", app)
	}

	for _, table := range tables {
		// Convert the table name to lowercase for case-insensitive comparison
		checkTableCmd := fmt.Sprintf(`%s -S %s,%s -U %s -P %s -d %s -Q "SET NOCOUNT ON; SELECT 1 WHERE OBJECT_ID('%s', 'U') IS NOT NULL;" -h -1 -W`, sqlcmdPathLocal, dbHost, dbPort, dbUser, dbPass, dbTmpEazybi, table)

		// Log the command being executed for table checking
		log.Printf("Executing command to check table existence: %s", checkTableCmd)

		output, err := runCommandRestoreOutput(checkTableCmd, envVars)
		if err != nil || strings.TrimSpace(output) != "1" {
			log.Printf("Table %s does not exist or error occurred, skipping...", table)
			continue
		}

		// Export the table data and create a format file (.fmt) directly on the remote SQL Server
		var dumpCmd, fmtCmd string
		if osType == "windows" {
			// For Windows, escape backslashes in the commands
			dumpCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s.dbo.%s out %s\\%s.bcp -c -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbTmpEazybi, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)

			// Create the .fmt file on the remote server
			fmtCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s.dbo.%s format nul -c -f %s\\%s.fmt -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbTmpEazybi, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		} else {
			// For Linux, normal forward slashes
			dumpCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s.dbo.%s out %s/%s.bcp -c -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbTmpEazybi, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)

			// Create the .fmt file on the remote server
			fmtCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "%s %s.dbo.%s format nul -c -f %s/%s.fmt -t, -r \n -S %s,%s -U %s -P %s"`, 
				serverPassword, remoteUser, dbHost, bcpPath, dbTmpEazybi, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		}

		// Execute the dump and format commands directly on the remote server
		if err := runCommandRestore(dumpCmd, nil); err != nil {
			log.Printf("Failed to dump table %s directly on SQL Server: %v", table, err)
			continue
		}
		if err := runCommandRestore(fmtCmd, nil); err != nil {
			log.Printf("Failed to create format file for table %s directly on SQL Server: %v", table, err)
			continue
		}
		
		// Truncate the existing table on SQL Server
		truncateCmd := fmt.Sprintf(`%s -S %s,%s -U %s -P %s -d %s -Q "TRUNCATE TABLE %s;"`, sqlcmdPathLocal, dbHost, dbPort, dbUser, dbPass, dbName, table)
		if err := runCommandRestore(truncateCmd, envVars); err != nil {
			log.Printf("Failed to truncate table %s: %v", table, err)
			continue
		}
				
		// Perform the bulk insert using the .fmt file
		var bulkInsertCmd string
		if osType == "windows" {
			// For Windows, properly escape backslashes and row terminators
			bulkInsertCmd = fmt.Sprintf(
				`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "bcp %s.dbo.%s in %s\\\\%s.bcp -f %s\\\\%s.fmt -S %s,%s -U %s -P %s"`,
				serverPassword, remoteUser, dbHost, dbName, table, remoteTempFolder, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		} else {
			// For Linux, use standard forward slashes
			bulkInsertCmd = fmt.Sprintf(
				`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "bcp %s.dbo.%s in %s/%s.bcp -f %s/%s.fmt -S %s,%s -U %s -P %s"`,
				serverPassword, remoteUser, dbHost, dbName, table, remoteTempFolder, table, remoteTempFolder, table, dbHost, dbPort, dbUser, dbPass)
		}

		if err := runCommandRestore(bulkInsertCmd, nil); err != nil {
			log.Printf("Failed to bulk insert table %s on SQL Server: %v", table, err)
			continue
		}
		// Step 1: Clean up the local .bcp file
		localBCPFile := fmt.Sprintf("%s/%s.bcp", localTempFolder, table)
		log.Printf("Removing local .bcp file: %s", localBCPFile)
		if err := os.Remove(localBCPFile); err != nil {
			log.Printf("Failed to remove local .bcp file for table %s: %v", table, err)
		} else {
			log.Printf("Successfully removed local .bcp file for table %s.", table)
		}
	 
		// Step 2: Clean up the remote .bcp .fmt files on the SQL Server
		var removeRemoteBCPCmd string
		var removeRemoteFMTCmd string

		if osType == "windows" {
			removeRemoteBCPCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "del %s\\%s.bcp"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)
			removeRemoteFMTCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "del %s\\%s.fmt"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)

		} else {
			removeRemoteBCPCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "rm -rf %s/%s.bcp"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)
			removeRemoteFMTCmd = fmt.Sprintf(`sshpass -p "%s" ssh -o StrictHostKeyChecking=no %s@%s "rm -rf %s/%s.fmt"`, serverPassword, remoteUser, dbHost, remoteTempFolder, table)

		}
	 
		log.Printf("Removing remote .bcp file for table %s: %s", table, removeRemoteBCPCmd)
		if err := runCommandRestore(removeRemoteBCPCmd, envVars); err != nil {
			log.Printf("Failed to remove remote .bcp or .fmt file for table %s: %v", table, err)
		} else {
			log.Printf("Successfully removed remote .bcp and .fmt file for table %s.", table)
		}

		log.Printf("Removing remote .fmt file for table %s: %s", table, removeRemoteFMTCmd)
		if err := runCommandRestore(removeRemoteBCPCmd, envVars); err != nil {
			log.Printf("Failed to remove remote .bcp or .fmt file for table %s: %v", table, err)
		} else {
			log.Printf("Successfully removed remote .bcp and .fmt file for table %s.", table)
		}

		log.Printf("Table %s processed successfully.", table)
	}

	return nil
}