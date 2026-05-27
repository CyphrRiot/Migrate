# Migrate OpenBSD Migration

This document tracks the port of the Migrate backup tool from Linux to OpenBSD.

---

## Table of Contents

1.  [Architecture Decisions](#architecture-decisions)
2.  [Completed Work](#completed-work)
3.  [Broken / Blocking](#broken--blocking)
4.  [Discovered Gotchas](#discovered-gotchas)
5.  [Subsystem-by-Subsystem Status](#subsystem-by-subsystem-status)
6.  [Testing Strategy](#testing-strategy)
7.  [Next Steps](#next-steps)

---

## Architecture Decisions

### Build Tags Over Runtime Checks

All OS-specific code lives in files with `//go:build` constraints (`_linux.go`, `_openbsd.go`). No `runtime.GOOS` switches in business logic.

### `internal/platform/` for Cross-Cutting OS Behaviors

-   `elevation_linux.go` / `elevation_openbsd.go` — `CheckPrivileges() PrivLevel`
-   `check_linux.go` / `check_openbsd.go` — `CheckRequirements()`
-   `deps_linux.go` / `deps_openbsd.go` — `CheckDependencies()`
-   `permissions.go` — `PrivLevel` enum (`PrivUser`, `PrivRoot`)

### `internal/app/` for Bootstrap Logic

`main.go` is a thin shim. Startup logic lives in `internal/app/`:

-   `lock.go` — PID-based instance lock using `os.TempDir()`
-   `terminal.go` — Terminal size via `golang.org/x/term`
-   `errors.go` — Error box rendering
-   `run.go` — Orchestrates lock, deps, signals, Bubble Tea init

### `internal/drives/` for Drive Subsystem Portability

Boundary for all block device, mount, and encryption logic. Linux-specific concepts (`lsblk`, `udisksctl`, `cryptsetup`, `/dev/mapper`, `/proc/mounts`) stay inside this package behind build tags. **No shell execution** for core mount/unmount on OpenBSD.

### Home Directory Resolution Must Be Portable

Never construct home directories by string concatenation. Use `os.UserHomeDir()` or `os/user.Lookup()` on `SUDO_USER`.

---

## Completed Work

### 1. Platform Abstractions

| File(s) | What |
|---|---|
| `diskstats_linux.go` / `diskstats_openbsd.go` | `GetDiskStats()` abstracts `syscall.Statfs_t` field differences |
| `mountinfo_linux.go` / `mountinfo_openbsd.go` | Linux uses `/proc/mounts`; OpenBSD uses `syscall.Getfsstat` |
| `detector_linux.go` / `detector_openbsd.go` / `detector_common.go` | Drive discovery; OpenBSD uses `Getfsstat` |
| `zerocopy_linux.go` / `zerocopy_openbsd.go` | OpenBSD falls back to `io.Copy` |
| `fallocate_linux.go` / `fallocate_openbsd.go` | OpenBSD is a no-op |
| `elevation_linux.go` / `elevation_openbsd.go` | `CheckPrivileges() PrivLevel` replaces `EnsureRoot()` |
| `deps_linux.go` / `deps_openbsd.go` | Linux validates `lsblk`/`udisksctl`/`cryptsetup`; OpenBSD returns `nil` |
| `platform/permissions.go` | `PrivUser` / `PrivRoot` enum |

### 2. `main.go` Refactored to Shim

-   `main.go` calls `platform.CheckPrivileges()` then `app.Run(perm)`
-   `internal/app/run.go` accepts `platform.PrivLevel` and passes it to `internal.InitialModel(perm)`
-   Lock file uses `os.TempDir()` instead of hardcoded `/tmp/migrate.lock`

### 3. Privilege-Adaptive Menus

-   `screens/menus.go` — `MainMenuChoicesNonRoot`, `BackupMenuChoicesNonRoot`, `GetMainMenuActionNonRoot`, `GetBackupMenuActionNonRoot`
-   `handlers/main_menu.go` — `NewMainMenuHandler(priv)` routes non-root index 1 to `"settings_backup"`
-   `handlers/backup_menu.go` — `NewBackupMenuHandler(priv)` routes non-root index 1 to `"settings_backup"`

### 4. Settings Backup Engine (`internal/operations.go`)

-   `startSettingsBackup(mountPoint string) tea.Cmd` — Bubble Tea command wrapper
-   `runSettingsBackupSilently(mountPoint string)` — background worker that:
    - Creates `<mountPoint>/migrate-settings-<hostname>-<date>/etc/`
    - Walks `/etc` recursively
    - Silently skips permission-denied files
    - Copies readable files via `copyFileEfficient()`
    - Writes `BACKUP-INFO.txt` with `"Backup Type: System Settings"`
    - Reports via `tuiBackupCompleted` / `tuiBackupError` / atomic counters
-   `sync/atomic` import added for `filesCopied` / `totalFilesFound`

### 5. Settings Backup Detection (`internal/operations.go`)

-   `detectBackupType()` recognizes `"Backup Type: System Settings"` in `BACKUP-INFO.txt` → returns `"settings"`
-   Fallback searches for `migrate-settings-*` directories and verifies their `BACKUP-INFO.txt`

### 6. Settings Restore Routing (`internal/operations.go`)

-   `startRestore()` gains `case "settings":` that locates `migrate-settings-*/etc/` subdirectory
-   Sets `actualSourcePath` to the `etc/` subdir, routes `actualTargetPath` to `/etc` or custom path

### 7. UI Mappings (`internal/ui.go`)

-   `formatOperationName()` — maps `"settings_backup"` → `"System Settings Backup"`, `"settings_restore"` → `"System Settings Restore"`
-   `renderProgress()` — `case "settings_backup":` shows source `/etc`, destination `m.selectedDrive`

### 8. Mount Hints (`internal/drives/mounter_*.go`)

-   `mounter_linux.go` — `GetMountHint()` returns `"Mount your drive with: udisksctl mount -b /dev/sdX1"`
-   `mounter_openbsd.go` — `GetMountHint()` returns `"Mount your drive with: doas mount /dev/sdXi /mnt"`

### 9. Removed Untagged Files

-   `internal/drives/mounter.go` → replaced by `mounter_linux.go` + `mounter_openbsd.go`
-   `internal/drives/luks.go` → replaced by `luks_linux.go` + `luks_stub_openbsd.go`

---

## Broken / Blocking

### Blocker A: `internal/model.go` Does Not Compile

**Status:** `make` fails right now.

**Errors:**
```
internal/model.go:1097:13: not enough arguments in call to handlers.NewMainMenuHandler
    have ()
    want (platform.PrivLevel)
internal/model.go:1123:13: not enough arguments in call to handlers.NewBackupMenuHandler
    have ()
    want (platform.PrivLevel)
```

**Root cause:** `model.go` was reverted to HEAD during this session after corrupted edits. It is missing ALL privilege-adaptive and settings wiring:

-   [ ] `privilege platform.PrivLevel` field in `Model` struct
-   [ ] `InitialModel(perm platform.PrivLevel)` signature (currently `InitialModel()`)
-   [ ] Pass `m.privilege` to `handlers.NewMainMenuHandler()` and `handlers.NewBackupMenuHandler()`
-   [ ] `BackupDriveStatus` handler — missing `else if backupType == "settings"` branch (settings restore confirmation gets overwritten by system restore block)
-   [ ] `ScreenConfirm` case — missing `"settings_backup"` and `"settings_restore"` branches
-   [ ] `ProgressUpdate` `msg.Done` — missing `PrivUser` unmount skip

**Fix priority:** This is the first thing to do in the next session.

### Blocker B: `internal/drives.go` Hardcodes `/dev/mapper/`

**Impact:** Encrypted drive TUI flows reference non-existent paths on OpenBSD.

**Locations:** `mountSelectedDrive` (~line 173), `mountDriveForOperation` (~line 226), `mountDriveForRestore` (~line 271).

**Path forward:** Move mapper path construction into `drives/luks_linux.go`. Facade should call into `drives` package instead of building paths.

### Blocker C: `internal/utils.go:DefaultMount` Hardcoded to `/run/media`

**Impact:** OpenBSD uses `/run/media` which does not exist.

**Path forward:** Create `internal/utils_linux.go` + `internal/utils_openbsd.go` with build-tagged `DefaultMountPoint() string`.

### Blocker D: `mounter_openbsd.go` References `GetDeviceFromProcMounts`

**Impact:** `resolveDriveDevice` calls `GetDeviceFromProcMounts(device)` which is `//go:build linux` only. This will cause a compile error on OpenBSD.

**Path forward:** Remove or conditionally compile the `GetDeviceFromProcMounts` call in `mounter_openbsd.go`.

### Blocker E: `detector_openbsd.go` Returns Empty UUIDs

**Impact:** Encrypted-drive detection may not map correctly (uses `"luks-" + UUID`).

**Path forward:** Use device name or mount point as stable identifier. UUIDs are optional for the "external vs. system" heuristic.

---

## Discovered Gotchas

### OpenBSD `syscall.Statfs_t` Field Prefixes

All fields prefixed with `F_`; string fields are `[16]int8`:

```go
// Linux
total := int64(stat.Blocks) * int64(stat.Bsize)

// OpenBSD
total := int64(stat.F_blocks) * int64(stat.F_bsize)
```

Conversion helper:

```go
func fsnToString(src []int8) string {
    out := make([]byte, 0, len(src))
    for _, c := range src {
        if c == 0 { break }
        out = append(out, byte(c))
    }
    return string(out)
}
```

### `MNT_WAIT` Is Not Exported in Go's `syscall`

Use raw value `1`:

```go
n, err := syscall.Getfsstat(buf, 1) // MNT_WAIT = 1
```

### `golang.org/x/sys/unix` Lacks OpenBSD `CopyFileRange` / `Sendfile`

Use build-tagged fallback to `io.Copy`.

---

## Subsystem-by-Subsystem Status

| Subsystem | File(s) | Status | Notes |
|---|---|---|---|
| **Drive Detection** | `detector_*.go`, `detector_common.go` | ✅ Completed | OpenBSD uses `Getfsstat` |
| **Mount Info** | `mountinfo_*.go` | ✅ Completed | OpenBSD uses `Getfsstat` |
| **Disk Stats** | `diskstats_*.go` | ✅ Completed | `GetDiskStats()` abstraction |
| **Zero-Copy** | `zerocopy_*.go` | ✅ Completed | Falls back to `io.Copy` on OpenBSD |
| **Preallocation** | `fallocate_*.go` | ✅ Completed | No-op on OpenBSD |
| **Platform Elevation** | `elevation_*.go` | ✅ Completed | `CheckPrivileges() PrivLevel` |
| **Dependency Check** | `deps_*.go` | ✅ Completed | OpenBSD returns `nil` |
| **Main.go Shim** | `main.go`, `internal/app/` | ✅ Completed | Passes `PrivLevel` through |
| **Privilege System** | `platform/permissions.go`, handlers, screens | ✅ Completed | `PrivUser`/`PrivRoot` menus |
| **Drive Mounter** | `mounter_linux.go`, `mounter_openbsd.go` | ✅ Completed | Build-tagged; OpenBSD uses syscalls |
| **LUKS** | `luks_linux.go`, `luks_stub_openbsd.go` | ✅ Completed | Build-tagged stubs |
| **Settings Backup Engine** | `internal/operations.go` | ✅ Completed | `startSettingsBackup`, `runSettingsBackupSilently` |
| **Settings Detection** | `internal/operations.go` | ✅ Completed | `detectBackupType` + fallback |
| **Settings Restore Routing** | `internal/operations.go` | ✅ Completed | `startRestore` `case "settings"` |
| **UI Name/Progress Mappings** | `internal/ui.go` | ✅ Completed | `formatOperationName`, `renderProgress` |
| **Mount Hints** | `mounter_*.go` | ✅ Completed | `GetMountHint()` on both platforms |
| **Model Wiring** | `internal/model.go` | ❌ **BROKEN** | Reverted to HEAD; does not compile against updated handlers |
| **Facade LUKS refs** | `internal/drives.go` | ❌ Blocked | Hardcodes `/dev/mapper/` |
| **Default Mount** | `internal/utils.go` | ❌ Blocked | Hardcoded `/run/media` |
| **OpenBSD Mounter** | `mounter_openbsd.go` | ❌ Blocked | Calls `GetDeviceFromProcMounts` (linux-only) |
| **OpenBSD UUIDs** | `detector_openbsd.go` | ❌ Blocked | Returns empty UUIDs |

---

## Testing Strategy

1.  **Build Test:** `make` must succeed.
2.  **Cross-Compilation:** `GOOS=openbsd GOARCH=amd64 go build ./...` must succeed.
3.  **Static Analysis:** `grep -r "exec.Command" internal/*_openbsd.go` must return nothing.
4.  **Manual Testing:**
    -   Run `doas bin/migrate`; verify TUI starts.
    -   Verify drive detection lists external drives under `/mnt/`.
    -   Test backup/restore to/from `/mnt/` targets.
    -   Verify unmount cleans up properly.

---

## Next Steps

### Immediate (Next Session)

1.  **Fix `internal/model.go` compilation.** Add `privilege platform.PrivLevel` field, update `InitialModel()` to accept `PrivLevel`, pass `m.privilege` to `NewMainMenuHandler()` and `NewBackupMenuHandler()`. Run `make`.
2.  **Wire `settings_backup` in `ScreenConfirm`.** Add `case "settings_backup":` calling `startSettingsBackup(m.selectedDrive)`. Run `make`.
3.  **Wire `settings_restore` in `ScreenConfirm`.** Add `case "settings_restore":` calling `startRestore(m.selectedDrive, targetPath, false, false)` where `targetPath` is `/etc` for root, `~/migrate-restored-etc/` for non-root. Run `make`.
4.  **Fix `BackupDriveStatus` settings branch.** Add `else if backupType == "settings"` before the system restore block so settings restore confirmation is not overwritten. Run `make`.
5.  **Skip unmount prompt for `PrivUser`.** In `ProgressUpdate` `msg.Done`, bypass `"unmount_backup"` when `m.privilege == platform.PrivUser`. Run `make`.

### After Model is Fixed

6.  **Show mount hints in `ui.go`.** In `renderDriveSelect()`, when `len(m.drives) == 0` and `m.privilege == platform.PrivUser`, display `drives.GetMountHint()`. Run `make`.
7.  **Fix `internal/drives.go`** facade — move `/dev/mapper/` path construction into `drives` package.
8.  **Make `DefaultMount` portable** via build-tagged files.
9.  **Fix `mounter_openbsd.go`** — remove `GetDeviceFromProcMounts` call.
10. **Fix `detector_openbsd.go`** — provide stable identifier fallback for empty UUIDs.

---

*Document updated for OpenBSD 7.9 port of Migrate.*
