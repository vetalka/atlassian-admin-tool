package main

import (
	"admin_tool/handlers" // Adjust the import path to match your project
	"context"             // For context cancellation
	"log"                 // For logging messages
	"net/http"
	"os"        // For OS-related functionality like signal handling
	"os/signal" // For signal handling
	"syscall"   // For handling system calls (like SIGINT and SIGTERM)
	"time"      // For setting timeouts
)

func main() {
	// Initialize the database
	handlers.InitDB("/adminToolBackupDirectory/environment.db")
	defer handlers.CloseDB()

	// Start the cron backup scheduler
	handlers.InitScheduler()
	defer handlers.StopScheduler()

	// Set up routes
	http.HandleFunc("/login", handlers.HandleLogin)
	http.HandleFunc("/create-user", handlers.HandleUserCreation)
	http.HandleFunc("/license-setup", handlers.HandleLicenseSetup)
	http.HandleFunc("/logout", handlers.HandleLogout)

	// Routes for SSO login, callback, and logout
	http.HandleFunc("/sso-login", handlers.HandleSSOLogin)
	http.HandleFunc("/sso-callback", handlers.HandleSSOCallback) // Called by IdP after successful login
	http.HandleFunc("/logout-sso", handlers.HandleSSOLogout)     // Handle SSO and app logout

	// Serve favicon separately as a PNG
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.png")
	})

	// Protect the home route with SetupMiddleware and AuthMiddleware
	http.HandleFunc("/", handlers.SetupMiddleware(handlers.AuthMiddleware(handlers.HandleHome)))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Dynamic routes for environment actions
	http.Handle("/environment/", handlers.SetupMiddleware(handlers.AuthMiddleware(http.HandlerFunc(handlers.HandleEnvironmentPage))))

	// Show environment details
	http.Handle("/environment/show/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Environment Parameters", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleShowEnvironment)).ServeHTTP,
	)))

	// Edit environment parameters
	http.Handle("/environment/edit/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Environment Parameters", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleUpdateEnvironmentForm)).ServeHTTP,
	)))

	// Update environment submissions
	http.Handle("/environment/update", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Environment Parameters", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleUpdateEnvironment)).ServeHTTP,
	)))

	// Routes for actions: restart, stop, start
	http.Handle("/environment/restart-app/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Restart", handlers.GetAppFromRequest)(handlers.HandleRestartAppAJAX()).ServeHTTP,
	)))
	http.Handle("/environment/get-config/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Specific Node Actions", handlers.GetAppFromRequest)(handlers.HandleGetConfigAJAX()).ServeHTTP,
	)))
	http.Handle("/environment/stop-app/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Stop", handlers.GetAppFromRequest)(handlers.HandleStopAppAJAX()).ServeHTTP,
	)))
	http.Handle("/environment/start-app/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Start", handlers.GetAppFromRequest)(handlers.HandleStartAppAJAX()).ServeHTTP,
	)))

	// Route for backup options
	http.Handle("/environment/backup/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Backup", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleBackupOptions)).ServeHTTP,
	)))
	http.Handle("/environment/start-backup/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Backup", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleStartBackup)).ServeHTTP,
	)))
	http.Handle("/environment/backup-progress/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Backup", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleBackupProgress)).ServeHTTP,
	)))
	http.Handle("/environment/backup-status/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Backup", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleBackupStatus)).ServeHTTP,
	)))

	// Route for restore options
	http.Handle("/environment/restore/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Restore", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleRestorePage)).ServeHTTP,
	)))
	http.Handle("/environment/start-restore/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Restore", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleStartRestore)).ServeHTTP,
	)))
	http.Handle("/environment/restore-progress/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Restore", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleRestoreProgress)).ServeHTTP,
	)))
	http.Handle("/environment/restore-status/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Restore", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleRestoreStatus)).ServeHTTP,
	)))
	http.Handle("/environment/restore/get-backup-dates", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Restore", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleGetBackupDates)).ServeHTTP,
	)))
	http.Handle("/environment/restore/get-backup-times", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.CheckPermissionMiddleware("Restore", handlers.GetAppFromRequest)(http.HandlerFunc(handlers.HandleGetBackupTimes)).ServeHTTP,
	)))

	// Route for My Account page
	http.HandleFunc("/my-account", handlers.SetupMiddleware(handlers.AuthMiddleware(handlers.HandleMyAccount)))

	// Route for adding environments
	http.HandleFunc("/add-environment", handlers.SetupMiddleware(handlers.AuthMiddleware(handlers.AdminOnlyMiddleware(handlers.HandleAddEnvironment))))

	// Route for completing environment setup (DB password entry)
	http.HandleFunc("/complete-env-setup", handlers.SetupMiddleware(handlers.AuthMiddleware(handlers.AdminOnlyMiddleware(handlers.HandleCompleteEnvSetup))))

	// Route for deleting environments
	http.HandleFunc("/delete-environment", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleDeleteEnvironment),
	)))

	// Route for managing users under /settings/users
	http.Handle("/settings/users", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUserManagement),
	)))
	// Route for redirect /settings
	http.Handle("/settings", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleSettingsRedirect),
	)))
	http.Handle("/settings/all-users", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUserList),
	)))
	http.Handle("/settings/groups", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleGroupList),
	)))
	http.Handle("/settings/users/edit", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleEditUser),
	)))
	http.Handle("/settings/users/update-group", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUpdateUserGroup),
	)))

	// Route for managing groups under /settings/users
	http.Handle("/settings/groups/edit", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleEditGroup),
	)))
	http.Handle("/settings/groups/update-actions", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUpdateGroupActions),
	)))
	http.Handle("/settings/groups/delete", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleDeleteGroup),
	)))

	// Route for Local Directory user management
	http.Handle("/settings/local-ad", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleLocalUserManagement),
	)))
	http.Handle("/settings/local-ad/add", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleAddLocalUser),
	)))
	http.Handle("/settings/users/local/delete", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleDeleteUser),
	)))
	http.Handle("/settings/users/local/update-password", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUpdatePassword),
	)))

	// Route for managing authentication methods under /settings/auth-methods/toggle
	http.Handle("/settings/auth-methods/toggle", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleAuthMethods),
	)))

	// Route for handling updates to authentication methods
	http.Handle("/settings/auth-methods/update", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUpdateAuthMethods),
	)))

	// Route for listing SSO configurations
	http.Handle("/settings/sso", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleSSOList),
	)))

	// Route for creating a new SSO configuration
	http.Handle("/settings/sso/create", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleSSOForm),
	)))

	// Route for editing an SSO configuration
	http.Handle("/settings/sso/edit", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleEditSSOForm),
	)))

	// Route to handle saving SSO configuration
	http.Handle("/settings/save-sso", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(http.HandlerFunc(handlers.HandleSaveSSO)),
	)))

	// Route to handle updateing SSO configuration
	http.Handle("/settings/update-sso", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(http.HandlerFunc(handlers.HandleUpdateSSO)),
	)))
	// Route for managing user directories
	http.Handle("/settings/user-directories", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUserDirectories),
	)))

	/*
		// Not in use anable only if will integrate ldap connection
		http.Handle("/settings/user-directories/add", handlers.SetupMiddleware(handlers.AuthMiddleware(
			handlers.AdminOnlyMiddleware(handlers.HandleAddUserDirectory),
		)))

		http.Handle("/settings/user-directories/edit", handlers.SetupMiddleware(handlers.AuthMiddleware(
			handlers.AdminOnlyMiddleware(handlers.HandleEditUserDirectory),
		)))
	*/

	// Route for updating the license under /settings/updatelicense
	http.Handle("/settings/updatelicense", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleDisplayLicense),
	)))

	// Routes for environment-specific actions with dynamic app handling
	http.Handle("/environment/get-config-ip/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleGetConfigForIP()),
	)))
	http.Handle("/environment/add-jvm-arg/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleAddJVMArg()),
	)))
	http.Handle("/environment/remove-jvm-arg/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleRemoveJVMArg()),
	)))
	http.Handle("/environment/change-jvm-min/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleChangeJVMMin()),
	)))
	http.Handle("/environment/change-jvm-max/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleChangeJVMMax()),
	)))

	// ── Cron backup scheduler routes ──────────────────────────────────────────
	http.Handle("/cron/policies/create", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleCreatePolicy),
	)))
	http.Handle("/cron/policies/update/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleUpdatePolicy),
	)))
	http.Handle("/cron/policies/delete/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleDeletePolicy),
	)))
	http.Handle("/cron/policies/toggle/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleTogglePolicy),
	)))
	http.Handle("/cron/policies/run/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleRunNow),
	)))
	http.Handle("/cron/policies/logs/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleGetLogs),
	)))
	http.Handle("/cron/policies/runs/", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleGetRunDetail),
	)))
	http.Handle("/cron/policies", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleListPolicies),
	)))

	// Route for cleanup backups
	//http.HandleFunc("/cleanup-backups", handlers.SetupMiddleware(handlers.AuthMiddleware(handlers.HandleCleanupBackupsPage)))
	http.Handle("/cleanup-backups", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleCleanupBackupsPage),
	)))
	http.Handle("/get-backup-dates", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleGetBackupDatesAndTimes),
	)))
	http.Handle("/delete-backups", handlers.SetupMiddleware(handlers.AuthMiddleware(
		handlers.AdminOnlyMiddleware(handlers.HandleDeleteSelectedBackups),
	)))

	/*
		// Route for popup message
		http.HandleFunc("/sse-backup-restore", handlers.HandleSSEBackupRestore)
	*/

	// Start the server
	// Create a new HTTP server instance
	srv := &http.Server{
		Addr:    ":8000",
		Handler: nil, // Use the default ServeMux
	}

	// Run server in a goroutine so that it doesn't block
	go func() {
		log.Println("Server is running on http://localhost:8000")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Set up a channel to listen for interrupt or terminate signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received
	<-quit
	log.Println("Shutting down the server...")

	// Create a context with a timeout to gracefully shut down the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Shut down the server gracefully
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exiting.")
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	// Serve your HTML template here
	http.ServeFile(w, r, "templates/favicon.html")
}
