package handlers

import (
	"database/sql"
	"fmt"
	"log"
	_ "modernc.org/sqlite" // SQLite driver
	"os"
	"path/filepath"
	"strings"
)

var db *sql.DB

// InitDB initializes the database connection and creates tables if necessary
func InitDB(dataSourceName string) {
	log.Printf("Initializing database at path: %s", dataSourceName)

	// Ensure directory exists
	dir := filepath.Dir(dataSourceName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create database directory: %v", err)
	}

	var err error
	db, err = sql.Open("sqlite", dataSourceName)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	// Test the connection
	err = db.Ping()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	log.Println("Database connection established.")

	// Create environments table if it doesn't exist
	query := `
    CREATE TABLE IF NOT EXISTS environments (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        app TEXT NOT NULL,
        name TEXT NOT NULL,
        ip TEXT NOT NULL,
        server_user TEXT NOT NULL,
        server_password TEXT NOT NULL,
        home_dir TEXT NOT NULL,
		install_dir TEXT NOT NULL,
        sharedhome_dir TEXT NOT NULL,
        app_dbname TEXT NOT NULL,
		app_dbuser TEXT NOT NULL,
		app_dbpass TEXT NOT NULL,
		app_dbport TEXT NOT NULL,
		app_dbhost TEXT NOT NULL,
        db_driver TEXT NOT NULL,
        eazybi_dbname TEXT NOT NULL,
		eazybi_dbuser TEXT NOT NULL,
		eazybi_dbpass TEXT NOT NULL,
		eazybi_dbport TEXT NOT NULL,
		eazybi_dbhost TEXT NOT NULL,
        base_url TEXT NOT NULL
    );
    `
	_, err = db.Exec(query)
	if err != nil {
		log.Fatalf("Failed to create environments table: %v", err)
	}

	// Migrate: add DB server credential columns if they don't exist
	migrationColumns := map[string]string{
		"db_server_user":     "TEXT NOT NULL DEFAULT ''",
		"db_server_password": "TEXT NOT NULL DEFAULT ''",
		"db_connection_type": "TEXT NOT NULL DEFAULT 'ssh'",
	}
	for col, colType := range migrationColumns {
		_, err = db.Exec(fmt.Sprintf("ALTER TABLE environments ADD COLUMN %s %s", col, colType))
		if err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				log.Printf("Migration note for column %s: %v", col, err)
			}
		} else {
			log.Printf("Added column %s to environments table", col)
		}
	}

	// Create license table if it doesn't exist
	licenseTableQuery := `
    CREATE TABLE IF NOT EXISTS license (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        key TEXT NOT NULL,
        expiry_date TEXT NOT NULL
    );
    `
	_, err = db.Exec(licenseTableQuery)
	if err != nil {
		log.Fatalf("Failed to create license table: %v", err)
	}

	// Create users table if it doesn't exist
	usersTableQuery := `
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        username TEXT NOT NULL UNIQUE,
        password TEXT NOT NULL,
        directory TEXT NOT NULL,
        groups TEXT
    );
    `
	_, err = db.Exec(usersTableQuery)
	if err != nil {
		log.Fatalf("Failed to create users table: %v", err)
	}

	// Create a unique index on the username field
	uniqueIndexQuery := `
    CREATE UNIQUE INDEX IF NOT EXISTS unique_username_idx ON users (username);
    `
	_, err = db.Exec(uniqueIndexQuery)
	if err != nil {
		log.Fatalf("Failed to create unique index on users table: %v", err)
	}

	// Create groups table if it doesn't exist
	groupsTableQuery := `
    CREATE TABLE IF NOT EXISTS groups (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        directory TEXT NOT NULL,
        groups TEXT
    );
    `
	_, err = db.Exec(groupsTableQuery)
	if err != nil {
		log.Fatalf("Failed to create groups table: %v", err)
	}

	// Insert default groups if they don't already exist based on group name
	insertGroupsQuery := `
    INSERT INTO groups (directory, groups) 
    SELECT 'Local Directory', 'administrators' 
    WHERE NOT EXISTS (SELECT 1 FROM groups WHERE groups = 'administrators');

    INSERT INTO groups (directory, groups) 
    SELECT 'Local Directory', 'developer' 
    WHERE NOT EXISTS (SELECT 1 FROM groups WHERE groups = 'developer');

    INSERT INTO groups (directory, groups) 
    SELECT 'Local Directory', 'user' 
    WHERE NOT EXISTS (SELECT 1 FROM groups WHERE groups = 'user');
    `
	_, err = db.Exec(insertGroupsQuery)
	if err != nil {
		log.Fatalf("Failed to insert default groups: %v", err)
	}

	log.Println("Default groups inserted successfully if not already present.")

	// Create actions table if it doesn't exist
	actionsTableQuery := `
    CREATE TABLE IF NOT EXISTS actions  (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        action TEXT NOT NULL,
        app TEXT NOT NULL
    );
    `
	_, err = db.Exec(actionsTableQuery)
	if err != nil {
		log.Fatalf("Failed to create actions table: %v", err)
	}

	// Insert default actions if they don't already exist
	insertActionsQuery := `
    INSERT INTO actions (action, app) 
    SELECT 'Restart Jira', 'jira' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Restart Jira' AND app = 'jira');

    INSERT INTO actions (action, app) 
    SELECT 'Stop Jira', 'jira' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Stop Jira' AND app = 'jira');

    INSERT INTO actions (action, app) 
    SELECT 'Start Jira', 'jira' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Start Jira' AND app = 'jira');

    INSERT INTO actions (action, app) 
    SELECT 'Backup Jira', 'jira' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Backup Jira' AND app = 'jira');

    INSERT INTO actions (action, app) 
    SELECT 'Restore Jira', 'jira' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Restore Jira' AND app = 'jira');

    INSERT INTO actions (action, app) 
    SELECT 'Restart Confluence', 'confluence' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Restart Confluence' AND app = 'confluence');

    INSERT INTO actions (action, app) 
    SELECT 'Stop Confluence', 'confluence' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Stop Confluence' AND app = 'confluence');

    INSERT INTO actions (action, app) 
    SELECT 'Start Confluence', 'confluence' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Start Confluence' AND app = 'confluence');

    INSERT INTO actions (action, app) 
    SELECT 'Backup Confluence', 'confluence' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Backup Confluence' AND app = 'confluence');

    INSERT INTO actions (action, app) 
    SELECT 'Restore Confluence', 'confluence' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Restore Confluence' AND app = 'confluence');

    INSERT INTO actions (action, app) 
    SELECT 'Restart Bitbucket', 'bitbucket' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Restart Bitbucket' AND app = 'bitbucket');

    INSERT INTO actions (action, app) 
    SELECT 'Stop Bitbucket', 'bitbucket' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Stop Bitbucket' AND app = 'bitbucket');

    INSERT INTO actions (action, app) 
    SELECT 'Start Bitbucket', 'bitbucket' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Start Bitbucket' AND app = 'bitbucket');

    INSERT INTO actions (action, app) 
    SELECT 'Backup Bitbucket', 'bitbucket' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Backup Bitbucket' AND app = 'bitbucket');

    INSERT INTO actions (action, app) 
    SELECT 'Restore Bitbucket', 'bitbucket' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Restore Bitbucket' AND app = 'bitbucket');

    INSERT INTO actions (action, app) 
    SELECT 'Environment Parameters Jira', 'jira' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Environment Parameters Jira' AND app = 'jira');

    INSERT INTO actions (action, app) 
    SELECT 'Environment Parameters Confluence', 'confluence' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Environment Parameters Confluence' AND app = 'confluence');

    INSERT INTO actions (action, app) 
    SELECT 'Environment Parameters Bitbucket', 'bitbucket' 
    WHERE NOT EXISTS (SELECT 1 FROM actions WHERE action = 'Environment Parameters Bitbucket' AND app = 'bitbucket');
    `
	_, err = db.Exec(insertActionsQuery)
	if err != nil {
		log.Fatalf("Failed to insert default actions: %v", err)
	}

	log.Println("Default actions inserted successfully if not already present.")

	// Create actions table if it doesn't exist
	groupactionsTableQuery := `
    CREATE TABLE IF NOT EXISTS group_actions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    group_name TEXT NOT NULL,
    action_id INTEGER NOT NULL,
    FOREIGN KEY (action_id) REFERENCES actions(id) ON DELETE CASCADE
    );
    `
	_, err = db.Exec(groupactionsTableQuery)
	if err != nil {
		log.Fatalf("Failed to create group_actions table: %v", err)
	}

	// Create sso_configuration table if it doesn't exist
	ssoTableQuery := `
    CREATE TABLE IF NOT EXISTS sso_configuration (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        config_name TEXT NOT NULL,
        sso_login_url TEXT NOT NULL,
        sso_logout_url TEXT,
        certificate TEXT,
        acs_url TEXT NOT NULL, -- Assertion Consumer Service URL
        audience_url TEXT, -- Audience URL
        username_mapping TEXT, -- Username Mapping
        jit_provisioning BOOLEAN DEFAULT 0, -- JIT Provisioning (Default: False)
        remember_user_logins BOOLEAN DEFAULT 0, -- Remembering User Logins (Default: False)
        login_button_text TEXT DEFAULT 'Continue with IdP', -- Login Button Text
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    `
	_, err = db.Exec(ssoTableQuery)
	if err != nil {
		log.Fatalf("Failed to create sso_configuration table: %v", err)
	}

	// Create auth_methods table if it doesn't exist
	authMethodTableQuery := `
    CREATE TABLE IF NOT EXISTS auth_methods (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    method_name TEXT UNIQUE NOT NULL,
    description TEXT,
    enabled BOOLEAN DEFAULT 0
    );
    `
	_, err = db.Exec(authMethodTableQuery)
	if err != nil {
		log.Fatalf("Failed to create auth_methods table: %v", err)
	}

	// Check if 'Username and Password' method already exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM auth_methods WHERE method_name = ?", "Username and Password").Scan(&count)
	if err != nil {
		log.Fatalf("Failed to query auth_methods table: %v", err)
	}

	// Insert 'Username and Password' only if it doesn't exist
	if count == 0 {
		authMethodInsertQuery := `
        INSERT INTO auth_methods (method_name, description, enabled) 
        VALUES ('Username and Password', 'Product login form', 1);
        `
		_, err = db.Exec(authMethodInsertQuery)
		if err != nil {
			log.Fatalf("Failed to insert into auth_methods table: %v", err)
		}
	}

	// Create backup_policies table if it doesn't exist
	backupPoliciesTableQuery := `
    CREATE TABLE IF NOT EXISTS backup_policies (
        id                 INTEGER  PRIMARY KEY AUTOINCREMENT,
        name               TEXT     NOT NULL,
        environment_id     INTEGER  REFERENCES environments(id),
        schedule           TEXT     NOT NULL,
        backup_types       TEXT     NOT NULL DEFAULT '[]',
        destination_folder TEXT     NOT NULL DEFAULT '',
        retention_days     INTEGER  NOT NULL DEFAULT 30,
        enabled            INTEGER  NOT NULL DEFAULT 1,
        created_at         DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at         DATETIME DEFAULT CURRENT_TIMESTAMP
    );
    `
	_, err = db.Exec(backupPoliciesTableQuery)
	if err != nil {
		log.Fatalf("Failed to create backup_policies table: %v", err)
	}

	// Create backup_policy_runs table if it doesn't exist
	backupPolicyRunsTableQuery := `
    CREATE TABLE IF NOT EXISTS backup_policy_runs (
        id                INTEGER  PRIMARY KEY AUTOINCREMENT,
        policy_id         INTEGER  REFERENCES backup_policies(id) ON DELETE CASCADE,
        started_at        DATETIME,
        finished_at       DATETIME,
        status            TEXT     NOT NULL DEFAULT 'running',
        log               TEXT     NOT NULL DEFAULT '',
        backup_size_bytes INTEGER  NOT NULL DEFAULT 0,
        files_created     TEXT     NOT NULL DEFAULT '[]'
    );
    `
	_, err = db.Exec(backupPolicyRunsTableQuery)
	if err != nil {
		log.Fatalf("Failed to create backup_policy_runs table: %v", err)
	}

	log.Println("Backup scheduler tables are ready.")

	log.Println("Database tables are ready.")
}

// CloseDB closes the database connection
func CloseDB() {
	if db != nil {
		err := db.Close()
		if err != nil {
			log.Fatalf("Failed to close database: %v", err)
		} else {
			log.Println("Database connection closed.")
		}
	}
}
