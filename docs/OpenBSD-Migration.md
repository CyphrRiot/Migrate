# Migrate OpenBSD Migration

This document tracks the port of the Migrate backup tool from Linux to OpenBSD. Migrate is a Go-based TUI application that performs system backup, restore, and verification operations. It uses [Bubble Tea](https://github.com/charmbracelet/bubbletea) for the interface and requires root privileges for drive mounting and filesystem operations.

---

## Table of Contents

1.  [Architecture Decisions](#architecture-decisions)
2.  [Completed Work](#completed-work)
3.  [Remaining Blockers](#remaining-blockers)
4.  [Discovered Gotchas](#discovered-gotchas)
5.  [Subsystem-by-Subsystem Status](#subsystem-by-subsystem-status)
6.  [Testing Strategy](#testing-strategy)
7.  [Next Steps](#next-steps)

---

## Architecture Decisions

### Build Tags Over Runtime Checks

All OS-specific code lives in files with `//go:build` constraints (`_linux.go`, `_openbsd.go`). This avoids `runtime.GOOS` switches scattered through business logic and makes the code cleaner at the package level.

Example pattern:

```go
//go:build linux
package drives
```

### `internal/platform/` for Cross-Cutting OS Behaviors

The `internal/platform` package holds OS-specific behaviors that do not belong in a single subsystem:

-   `elevation_linux.go` / `elevation_openbsd.go` — Root privilege gating (`EnsureRoot()`)
-   `check_linux.go` / `check_openbsd.go` — Runtime environment validation (`CheckRequirements()`)
-   `deps_linux.go` / `deps_openbsd.go` — Dependency validation (`CheckDependencies()`)

New platform-specific abstractions should be added here.

### `internal/app/` for Bootstrap Logic

The `main.go` entry point must remain a thin shim. All startup logic — singleton locking, terminal sizing, error rendering, signal handling, and TUI initialization — lives in `internal/app/`:

-   `lock.go` — PID-based instance lock using `os.TempDir()`
-   `terminal.go` — Terminal size detection via `golang.org/x/term`
-   `errors.go` — Responsive error box rendering and text wrapping
-   `run.go` — `Run()` orchestrates lock, dependency check, signals, and Bubble Tea init

### `internal/drives/` for Drive Subsystem Portability

The drives package is the boundary for all block device, mount, and encryption logic. All Linux-specific concepts (`lsblk`, `udisksctl`, `cryptsetup`, `/dev/mapper`, `/proc/mounts`) must remain inside this package and be abstracted behind build-tagged files. **No shell execution** is allowed for core mount/unmount functionality.

### Home Directory Resolution Must Be Portable

Never construct home directories by string concatenation (`"/home/" + username`). Always use `os.UserHomeDir()` or `os/user.Lookup()` to resolve the actual home directory for a user. This is critical for correct behavior when running under `sudo`/`doas` (via `SUDO_USER`).

---

## Completed Work

### 1. `syscall.Statfs_t` Field Names

**Resolution:** `internal/drives/diskstats_linux.go` and `diskstats_openbsd.go` export `GetDiskStats(path string) (total, free, avail int64, err error)`.

### 2. `syscall.Fallocate`, `unix.CopyFileRange`, `syscall.Sendfile`

**Resolution:**
-   `internal/fallocate_linux.go` / `fallocate_openbsd.go`
-   `internal/zerocopy_linux.go` / `zerocopy_openbsd.go`
-   `internal/filesystem.go` calls `tryFallocate`, `tryCopyFileRange`, `trySendfile`

### 3. `/proc/mounts` Dependency

**Resolution:** `mountinfo_linux.go` (keeps `/proc/mounts` parsing) and `mountinfo_openbsd.go` (uses `syscall.Getfsstat`).

### 4. `internal/platform/` Elevation

**Resolution:** `elevation_linux.go` prints `sudo migrate`; `elevation_openbsd.go` prints `doas migrate`. `main.go` calls `platform.EnsureRoot()`.

### 5. `detector.go` Drive Discovery

**Resolution:**
-   `detector_linux.go` — `lsblk` JSON (existing)
-   `detector_openbsd.go` — `syscall.Getfsstat`
-   `detector_common.go` — shared `isExternalMount()`

### 6. Hardcoded `grendel` and `/home/` Paths

**Resolution:** Replaced all hardcoded paths with `getHomeDir()` using `os/user.Lookup` on `SUDO_USER`. Fixed in `internal/operations.go`, `internal/utils.go`, `internal/drives/utils.go`, `internal/verification.go`, `internal/drives/luks.go`.

### 7. Dependency Check Extraction from `main.go`

**Resolution:** `internal/platform/deps_linux.go` validates `lsblk`, `udisksctl`, `cryptsetup`, `umount`. `internal/platform/deps_openbsd.go` returns `nil`.

### 8. `main.go` Refactored to Shim

**Resolution:**
-   Created `internal/app/` package with `lock.go`, `terminal.go`, `errors.go`, `run.go`
-   `main.go` is now 13 lines: calls `platform.EnsureRoot()` then `app.Run()`
-   Lock file path uses `os.TempDir()` instead of hardcoded `/tmp/migrate.lock`

---

## Remaining Blockers

### Blocker 1: `internal/drives/mounter.go` Shells Out to Binaries

**Impact:** Mount and unmount operations fail on OpenBSD.

**Details:**
-   `MountRegularDrive` → `mountDevice` calls `exec.Command("udisksctl", "mount", ...)`
-   `UnmountBackupDrive` calls `exec.Command("sudo", "umount", ...)` and `exec.Command("sudo", "cryptsetup", "close", ...)`
-   `resolveDriveDevice` calls `GetDeviceFromProcMounts()` which is `linux`-tagged only

**Path forward:** Split into `mounter_linux.go` + `mounter_openbsd.go`. OpenBSD uses `syscall.Mount()` / `syscall.Unmount()` directly.

### Blocker 2: `internal/drives/luks.go` Has No Build Tag

**Impact:** Compiles on both platforms but runtime-fails on OpenBSD.

**Details:** References `/dev/mapper`, `udisksctl unlock`, `cryptsetup`, `crypto_LUKS`. OpenBSD uses `softraid`/`bioctl`.

**Path forward:** Rename to `luks_linux.go` with `//go:build linux`. Create `luks_openbsd.go` with no-op stubs.

### Blocker 3: `internal/drives.go` Facade Hardcodes `/dev/mapper/`

**Impact:** Encrypted drive TUI flows reference non-existent paths on OpenBSD.

**Path forward:** Move mapper path construction into `internal/drives/luks_linux.go`. Facade should call into `drives` package instead of building paths.

### Blocker 4: `internal/utils.go:DefaultMount` Hardcoded to `/run/media`

**Impact:** OpenBSD mount operations use incorrect default paths.

**Path forward:** Create `internal/utils_linux.go` + `internal/utils_openbsd.go` (or `internal/platform/`) with `DefaultMountPoint() string`.

### Blocker 5: `internal/drives/detector_openbsd.go` Returns Empty UUIDs

**Impact:** Encrypted-drive detection may not map correctly (uses `"luks-" + UUID`).

**Path forward:** Use device name or mount point as stable identifier. UUIDs are optional for the "external vs. system" heuristic.

### Blocker 6: UI Border Width Off-by-One on Emoji Menu Items

**Impact:** Right border misaligned by ~1 character on deselected and selected emoji-text menu items (`⚙️ Restore Settings`, `ℹ️ About`, `❌ Exit`).

**Details:** `lipgloss` width calculation does not account for zero-width joiner emoji sequences (e.g., `⚙️` is `U+2699 U+FE0F`, 2 runes but 1 cell). `github.com/mattn/go-runewidth` handles this, but may need explicit `lipgloss.WithWidthCells` or manual width adjustment.

**Path forward:** Verify `go-runewidth` is up to date (`v0.0.16` is current). If still broken, add manual `lipgloss.Style.Width()` override based on visible cell count rather than byte/rune count.

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

### `SUDO_USER` Home Directory Resolution

```go
if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
    if u, err := user.Lookup(sudoUser); err == nil {
        return u.HomeDir
    }
}
return os.UserHomeDir()
```

---

## Subsystem-by-Subsystem Status

| Subsystem | File(s) | Status | Notes |
|---|---|---|---|
| **Drive Detection** | `detector_linux.go`, `detector_openbsd.go`, `detector_common.go` | ✅ Completed | OpenBSD uses `Getfsstat` |
| **Mount Info** | `mountinfo_linux.go`, `mountinfo_openbsd.go` | ✅ Completed | OpenBSD uses `Getfsstat` |
| **Disk Stats** | `diskstats_linux.go`, `diskstats_openbsd.go` | ✅ Completed | `GetDiskStats()` abstraction |
| **Space Validation** | `space.go` | ✅ Completed | Uses `GetDiskStats()` |
| **Zero-Copy** | `zerocopy_linux.go`, `zerocopy_openbsd.go` | ✅ Completed | Falls back to `io.Copy` on OpenBSD |
| **Preallocation** | `fallocate_linux.go`, `fallocate_openbsd.go` | ✅ Completed | No-op on OpenBSD |
| **Platform Elevation** | `elevation_linux.go`, `elevation_openbsd.go` | ✅ Completed | `EnsureRoot()` |
| **Platform Requirements** | `check_linux.go`, `check_openbsd.go` | ✅ Completed | Validates `/proc`, `/sys` on Linux |
| **Dependency Check** | `deps_linux.go`, `deps_openbsd.go` | ✅ Completed | OpenBSD returns `nil` |
| **Home Dir Resolution** | `operations.go`, `utils.go`, `drives/utils.go` | ✅ Completed | `user.Lookup` for `SUDO_USER` |
| **Main.go Shim** | `main.go`, `internal/app/` | ✅ Completed | 13-line entry point |
| **UI Border Width** | `internal/ui.go` | 🔄 Known | Off-by-one on emoji menu items |
| **Drive Mounter** | `mounter.go` | ❌ Blocked | Still shells out to `udisksctl`, `sudo umount` |
| **LUKS** | `luks.go` | ❌ Blocked | No build tag; Linux-only internals |
| **Default Mount** | `utils.go` | ❌ Blocked | Hardcoded `/run/media` |
| **Facade LUKS refs** | `internal/drives.go` | ❌ Blocked | Hardcodes `/dev/mapper/` |

---

## Testing Strategy

1.  **Build Test:** `make` must succeed after each change.
2.  **Static Analysis:** `grep -r "exec.Command" internal/*_openbsd.go` must return nothing.
3.  **Manual Testing:**
    -   Run `doas bin/migrate`; verify TUI starts without dependency errors.
    -   Verify drive detection lists external drives under `/mnt/`.
    -   Test backup/restore to/from `/mnt/` targets.
    -   Verify unmount cleans up properly.
4.  **Cross-Compilation:** `GOOS=linux GOARCH=amd64 go build ./...` must succeed.

---

## Next Steps

In priority order:

1.  **Fix UI border width** on emoji menu items — investigate `lipgloss` width vs. `go-runewidth` cell count.
2.  **Split `mounter.go`** into `mounter_linux.go` + `mounter_openbsd.go` with `syscall.Mount`/`Unmount`.
3.  **Tag `luks.go`** as `linux`-only and create `luks_openbsd.go` no-op stubs.
4.  **Fix `internal/drives.go`** facade — move `/dev/mapper/` path construction into `drives` package.
5.  **Make `DefaultMount` portable** via build-tagged files.

---

*Document updated for OpenBSD 7.9 port of Migrate.*
