# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is an **Atlassian Admin Tool** — a Go web application for managing Atlassian environments (Jira, Confluence, Bitbucket, EazyBI). It provides backup/restore, application start/stop/restart, user management, RBAC, and SSO/SAML support via a browser UI on port 8000.

## Build & Run

```bash
# First-time setup (resolves WinRM dependency, tidies modules, builds)
./setup.sh

# Or manually:
go get github.com/masterzen/winrm@latest
go mod tidy
go build -o admin_tool .
./admin_tool
```

The binary writes its database to `/adminToolBackupDirectory/environment.db` on first run.

## Architecture

### Entry Point & Routing (`main.go`)
Registers 60+ HTTP routes and assembles middleware chains. Server runs on `:8000` with graceful shutdown on SIGINT/SIGTERM.

**Middleware stack (applied per route):**
1. `SetupMiddleware` — redirects to `/license-setup` or `/create-user` if initial setup incomplete
2. `AuthMiddleware` — session-based auth; redirects to `/login` if unauthenticated
3. `AdminOnlyMiddleware` — restricts route to admin users
4. `CheckPermissionMiddleware(action, app)` — dynamic RBAC check against DB

### Handlers (`handlers/`)
All business logic lives here (~58 files, ~13,300 lines). Key files:
- `init_db.go` — SQLite schema initialization
- `middleware.go` — all middleware implementations
- `templates.go` — `RenderPage()` central render function + base HTML layout
- `login.go` / `sso.go` — local auth and SAML SSO
- `HandleAJAX.go` — async endpoints (restart/stop/start app)
- `app_backup.go`, `app_restore.go`, `database_backup.go`, `database_restore.go` — backup/restore logic

### Database (SQLite)
Key tables: `environments`, `users`, `groups`, `actions`, `group_actions`, `license`, `sso_configuration`, `auth_methods`.

RBAC is enforced by mapping groups → actions (Restart, Stop, Start, Backup, Restore, etc.) per app (jira, confluence, bitbucket).

### Frontend
HTML is generated inline in Go handlers via `fmt.Sprintf()`. The `RenderPage()` function in `templates.go` wraps content in the base layout. Static assets (CSS, JS) are served from `static/`.

### Remote Execution
Application control (start/stop/restart) uses either SSH or WinRM depending on the environment's `connection_type` field.

## Key Dependencies
- `gorilla/sessions` — cookie-based session management
- `golang.org/x/crypto` — bcrypt password hashing
- `modernc.org/sqlite` — SQLite driver (pure Go, no CGO)
- `russellhaering/gosaml2` + `goxmldsig` — SAML SSO
- `masterzen/winrm` — Windows remote management
- `go-sql-driver/mysql`, `lib/pq`, `microsoft/go-mssqldb` — managed app DB connectors
