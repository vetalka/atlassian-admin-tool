package handlers

import (
    "sync"
)

// Task structure to track progress and message for a task (backup, restore)
type Task struct {
    Progress int
    Message  string
}

var (
    taskStatus = make(map[string]*Task)
    mu         sync.Mutex // Mutex to synchronize access to taskStatus
)

// AddTask adds a new task for the given environment
func AddTask(env string) {
    mu.Lock()
    defer mu.Unlock()

    taskStatus[env] = &Task{Progress: 0, Message: "Starting..."}
}

// UpdateTaskProgress updates the progress and message of a task for the given environment
func UpdateTaskProgress(env string, progress int, message string) {
    mu.Lock()
    defer mu.Unlock()

    if task, exists := taskStatus[env]; exists {
        task.Progress = progress
        task.Message = message
    }
}

// GetTaskStatus returns the progress and message of the current task for a specific environment
func GetTaskStatus(environmentName string) (int, string) {
    if status, exists := backupStatus[environmentName]; exists {
        return status.Progress, status.Message
    }
    return 0, "No backup in progress."
}

// GetRestoreTaskStatus returns the progress and message of the restore task for a specific environment
func GetRestoreTaskStatus(environmentName string) (int, string) {
    if status, exists := restoreStatus[environmentName]; exists {
        return status.Progress, status.Message
    }
    return 0, "No restore in progress."
}
