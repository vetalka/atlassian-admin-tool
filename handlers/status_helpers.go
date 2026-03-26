package handlers

/*
import "sync"
// RestoreStatus holds the progress and message of the restore process
type RestoreStatus struct {
    Progress int    // Percentage of completion
    Message  string // Status message
}

// Global map to hold status for different environments
var restoreStatus = make(map[string]RestoreStatus)

// Mutex to prevent concurrent map access issues
var restoreStatusMutex sync.Mutex

// Function to update restore status safely
func UpdateRestoreStatus1(environment string, progress int, message string) {
    restoreStatusMutex.Lock()
    restoreStatus[environment] = RestoreStatus{Progress: progress, Message: message}
    restoreStatusMutex.Unlock()
}

// Function to get restore status safely
func GetRestoreStatus(environment string) (RestoreStatus, bool) {
    restoreStatusMutex.Lock()
    status, exists := restoreStatus[environment]
    restoreStatusMutex.Unlock()
    return status, exists
}
*/
