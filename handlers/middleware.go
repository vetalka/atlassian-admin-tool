package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/sessions"
)

// Initialize the session store
var store *sessions.CookieStore

func init() {
	secret := getSessionSecret()
	store = sessions.NewCookieStore([]byte(secret))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400,
		HttpOnly: true,
		Secure:   os.Getenv("SESSION_SECURE") != "false",
		SameSite: http.SameSiteLaxMode,
	}
}

// getSessionSecret returns the session signing key from SESSION_SECRET env var,
// falling back to a randomly generated key (sessions will be invalidated on restart).
func getSessionSecret() string {
	if secret := os.Getenv("SESSION_SECRET"); secret != "" {
		return secret
	}
	log.Println("WARNING: SESSION_SECRET env var not set; using a random key (sessions will not survive restarts)")
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("failed to generate random session secret: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// SetupMiddleware ensures that the license is set up and a user is created
func SetupMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Check if license is set up
        if !IsLicenseSetUp() {
            http.Redirect(w, r, "/license-setup", http.StatusSeeOther)
            return
        }

        // Check if any user is created
        var userCount int
        err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
        if err != nil {
            http.Error(w, "Internal Server Error", http.StatusInternalServerError)
            return
        }

        if userCount == 0 {
            http.Redirect(w, r, "/create-user", http.StatusSeeOther)
            return
        }

        // Call the next handler if everything is set up
        next.ServeHTTP(w, r)
    }
}

// AuthMiddleware checks if a user is authenticated before accessing a protected route
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        session, _ := store.Get(r, "session-name")

        // Check if user is authenticated
        if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
            // If not authenticated, redirect to login page
            log.Println("User not authenticated, redirecting to login.")
	    http.Redirect(w, r, "/login", http.StatusSeeOther)
            return
        }

	log.Println("User authenticated, accessing protected route.")
        // Call the next handler if authenticated
        next.ServeHTTP(w, r)
    }
}

// HandleLogout handles user logout
func HandleLogout(w http.ResponseWriter, r *http.Request) {
    session, _ := store.Get(r, "session-name")

    // Revoke user authentication
    session.Values["authenticated"] = false
    session.Save(r, w)

    // Redirect to the login page after logging out
    http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// AdminOnlyMiddleware ensures only admin users can access a route
func AdminOnlyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the current logged-in username
		username, err := GetCurrentUsername(r)
		if err != nil {
			log.Printf("Failed to get current user: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check if the user is an admin
		isAdmin, err := IsAdminUser(username)
		if err != nil {
			log.Printf("Failed to check if user is admin: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !isAdmin {
			RenderErrorPage(w, r, "Access Denied", "You don't have permission to access this page. Please contact your administrator if you need access.", "/", "Back to Home", http.StatusForbidden)
			return
		}

		// Proceed with the next handler if the user is an admin
		next.ServeHTTP(w, r)
	}
}

// CheckActionPermission is middleware that verifies if a user has permission to perform a specific action
func CheckActionPermission(action string, app string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Get the current logged-in username
            username, err := GetCurrentUsername(r)
            if err != nil {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            // Get allowed actions for the user
            allowedActions, err := GetCurrentActionsForUser(username)
            if err != nil {
                http.Error(w, "Failed to retrieve user actions", http.StatusInternalServerError)
                return
            }

            // Check if the user has permission for the specified action and app
            actionKey := fmt.Sprintf("%s %s", action, app)
            if !allowedActions[actionKey] {
                http.Error(w, "Forbidden: You don't have permission to perform this action", http.StatusForbidden)
                return
            }

            // If the user has permission, proceed to the next handler
            next.ServeHTTP(w, r)
        })
    }
}

// CheckPermissionMiddleware dynamically verifies if a user has permission to perform a specific action with a dynamic app
func CheckPermissionMiddleware(action string, getAppFromRequest func(*http.Request) (string, error)) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Get the current logged-in username
            username, err := GetCurrentUsername(r)
            if err != nil {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            // Check if the user is an admin
            isAdmin, err := IsAdminUser(username)
            if err != nil {
                http.Error(w, "Internal Server Error", http.StatusInternalServerError)
                return
            }

            // If the user is an admin, grant all permissions
            if isAdmin {
                next.ServeHTTP(w, r)
                return
            }

            // Dynamically fetch the app based on the request
            app, err := getAppFromRequest(r)
            if err != nil {
                http.Error(w, "Invalid request", http.StatusBadRequest)
                return
            }

            // Get the user's allowed actions
            allowedActions, err := GetCurrentActionsForUser(username)
            if err != nil {
                http.Error(w, "Failed to get user actions", http.StatusInternalServerError)
                return
            }

            // Construct the action key based on the app and action
            actionKey := fmt.Sprintf("%s %s", action, app)

            // Check if the user has permission for this action
            if !allowedActions[actionKey] {
                RenderErrorPage(w, r, "Access Denied", "You don't have permission to perform this action. Please contact your administrator.", "/", "Back to Home", http.StatusForbidden)
                return
            }

            // If permission is granted, proceed to the next handler
            next.ServeHTTP(w, r)
        })
    }
}


// GetAppFromRequest extracts the app based on the URL path or other parameters
func GetAppFromRequest(r *http.Request) (string, error) {
	// Extract the environment name from the URL path
	// Assuming the path structure is "/environment/[some-action]/<environment-name>"
	// We will split by '/' and find the last segment as the environment name

	// Get the path parts
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid request URL structure")
	}

	// Assume the environment name is the last segment in the URL
	environmentName := parts[len(parts)-1]
	if environmentName == "" {
		return "", fmt.Errorf("environment name not found in request URL")
	}

	// Retrieve the app name based on the environment name from the database
	var appName string
	err := db.QueryRow("SELECT app FROM environments WHERE name = ?", environmentName).Scan(&appName)
	if err != nil {
		return "", fmt.Errorf("app not found for environment: %s", environmentName)
	}

	return appName, nil
}
