# Migrate v1.0.4
<!-- Version is defined in version.go - update there to change everywhere -->

A stunningly beautiful **TUI-only** backup and restore tool built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss), featuring Tokyo Night inspired theming and **pure Go implementation with zero external dependencies**.

Now with **full `rsync --delete` equivalent functionality** including SHA256-based file deduplication and automatic cleanup of deleted files!

![Migrate Beautiful Interface](images/interface.gif)

## ⚠️ Warning: 
> ⚠️ This is new software with significant improvements over the bash version. While extensively tested, please ensure you have important data backed up elsewhere before use. Test backup and restore operations in non-critical environments first. ⚠️

> 🚨 **Major Update**: The tool now works with **any external drive** instead of being hardcoded to specific drives. Select your backup drive from the available options.

## 🎉 Pure Go Implementation

This tool now features a **complete pure Go backup implementation** with **zero external dependencies**. No more rsync, df, or shell commands during backup operations - just a single static binary that handles everything!

## ✨ Features

- 🎨 **Absolutely Beautiful TUI** - Tokyo Night color scheme with smooth animations  
- 🎯 **TUI-Only Interface** - No confusing CLI options, just beautiful UI always
- 🚀 **Complete System Backup** - 1:1 backup of your entire system using pure Go
- 🏠 **Home Directory Backup** - Selective backup of personal files
- 🔄 **System Restore** - Full system restoration from backup
- ⚡ **Pure Go Performance** - Efficient file copying without external dependencies
- 📊 **Real-time Progress** - Accurate progress tracking based on actual disk usage
- 💾 **External Drive Support** - Works with any external drive, automatic detection
- 🔒 **LUKS Support** - Full support for encrypted drives
- 🎯 **Smart Progress Calculation** - Accounts for existing backup data immediately
- 🔧 **Static Binary** - Single executable with no dependencies
- ✨ **NEW: rsync --delete Equivalent** - Full synchronization with file deletion support
- 🔍 **NEW: SHA256 File Deduplication** - Skip identical files for faster incremental backups

## ⚡ rsync --delete Equivalent

### Full Synchronization Support
- **✅ Delete removed files** - Files deleted from source are removed from backup
- **✅ Skip identical files** - SHA256 hash comparison prevents unnecessary copying
- **✅ Incremental backups** - Only copy changed/new files after first backup
- **✅ True synchronization** - Backup exactly matches source filesystem

### Advantages Over Traditional rsync
- **Cryptographic verification** - SHA256 provides stronger integrity than timestamp comparison
- **Zero dependencies** - No need for rsync binary or shell commands
- **Better logging** - Detailed statistics on copied vs. skipped files
- **Consistent behavior** - Same results regardless of system configuration

### Equivalent Command
Your Migrate tool now performs the equivalent of:
```bash
rsync -aAx --delete / /backup/destination/
```

But with better file verification and a beautiful TUI interface!

## 🚀 Pure Go Architecture

### Zero Dependencies
- **No rsync required** - Pure Go file copying with `io.Copy` and `filepath.WalkDir`
- **No shell commands** - Direct filesystem operations using `syscall.Statfs()`
- **Static binary** - Built with `CGO_ENABLED=0` for maximum portability
- **Efficient implementation** - Filesystem traversal respects boundaries and handles errors gracefully

### Technical Advantages
- **Fast startup** - No external process spawning
- **Reliable** - No dependency on system tools
- **Portable** - Single binary works anywhere
- **Memory efficient** - Direct file operations without shell overhead
- **Error resilient** - Continues operation even when individual files fail

## 📊 Perfect Progress Tracking

### Smart Progress Calculation
The tool intelligently calculates progress by measuring actual disk space usage:

```
Progress = current_destination_usage / total_source_size
```

### Immediate Progress Display
- **Accounts for existing backup data** - If destination has 800GB, shows ~44% immediately
- **Real-time updates** - Progress updates every 200ms (5x per second)
- **Session tracking** - Shows how much copied in current session
- **Accurate time estimation** - Based on current copy rate

### Example Progress Display
```
Copying 1.78 TB / 1.82 TB (+24.5 GB this session) (Est 2h 15m)
Progress: [████████████████████████████████████████░░░░░░░░] 98.2%
```

## 🎨 Beautiful Interface

The interface features:
- **Tokyo Night theme** - Professionally designed color scheme
- **Smooth progress bars** - Real-time visual feedback
- **Clear status messages** - Always know what's happening
- **Responsive layout** - Adapts to any terminal size

## 🚀 Installation

### Download Binary (Recommended)

```bash
# Download the standalone binary
curl -L -o migrate https://github.com/CyphrRiot/Migrate/raw/main/bin/migrate

# Make it executable
chmod +x migrate

# Install to your local bin (optional)
mv migrate ~/.local/bin/
```

### Build From Source

```bash
# Clone repository
git clone https://github.com/CyphrRiot/Migrate.git
cd Migrate

# Build static binary (pure Go, zero dependencies)
CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/migrate .

# Install
cp bin/migrate ~/.local/bin/
```

## 🖥️ Usage

### Beautiful TUI Interface (Only Mode)
```bash
sudo migrate
```

That's it! The tool always launches the beautiful TUI interface - no CLI options needed.

### What You'll See
- 🎨 **Stunning Tokyo Night interface** with smooth animations
- 📱 **Simple menu system** - navigate with arrow keys and Enter
- 🔍 **Automatic drive detection** - only shows external/removable drives
- 📊 **Real-time progress** - watch your backup progress in real-time

## ⚙️ How It Works

### 1. Pure Go Backup Process
```go
// Efficient directory synchronization
func syncDirectories(src, dst string) error {
    return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
        // Handle directories, files, and symlinks
        // Preserve permissions, ownership, timestamps
        // Respect filesystem boundaries (-x behavior)
        // Skip excluded patterns efficiently
    })
}
```

### 2. Real-time Progress Tracking
```go
// Accurate disk space measurement
func getUsedDiskSpace(path string) (int64, error) {
    var stat syscall.Statfs_t
    syscall.Statfs(path, &stat)
    
    totalBytes := int64(stat.Blocks) * int64(stat.Bsize)
    freeBytes := int64(stat.Bfree) * int64(stat.Bsize)
    return totalBytes - freeBytes, nil
}
```

### 3. Smart Drive Detection
```go
// Parse /proc/mounts for pure Go mount detection
func checkAnyBackupMounted() (string, bool) {
    data, _ := os.ReadFile("/proc/mounts")
    // Parse mount information without external commands
}
```

## 🚫 Automatic Exclusions

The following paths are automatically excluded for safety:
- `/dev/*` - Device files
- `/proc/*` - Process information  
- `/sys/*` - System information
- `/tmp/*` & `/var/tmp/*` - Temporary files
- `/lost+found` - Recovery directory
- **Backup destination** - Prevents infinite recursion

## 💾 External Drive Support

### Universal Compatibility
- **Any external drive** - USB, SSD, HDD automatically detected
- **LUKS encryption** - Seamless encrypted drive support
- **Multiple filesystems** - ext4, btrfs, exfat, NTFS support
- **Smart detection** - Only shows removable/hotpluggable devices

### Drive Selection Interface
```
💾 Select External Drive:
   /dev/sdb1 (32GB) - MyUSBDrive [ext4]
 → /dev/sdc (120GB) - BackupDrive [btrfs, ENCRYPTED]  
   /dev/sdd2 (1TB) - MyStorage [exfat]
```

## 📊 Progress Features

### Real-time Updates
- **200ms refresh rate** - Smooth, responsive progress tracking
- **Accurate percentages** - Based on actual filesystem usage
- **Time estimation** - Realistic completion time based on copy rate
- **Session tracking** - Shows progress since backup started

### Smart Initial Display
- **Immediate progress** - Shows correct percentage from start
- **Existing data awareness** - Accounts for previous backups
- **Clear messaging** - Always know what's happening

## 🏗️ Architecture

### Improved File Structure (v1.0.3)
```
├── main.go           # Entry point and TUI initialization
├── internal/         # Internal package (prevents external imports)
│   ├── version.go    # Centralized version management
│   ├── utils.go      # Utility functions and configuration
│   ├── filesystem.go # File operations and disk management
│   ├── drives.go     # Drive detection and mounting
│   ├── operations.go # Backup and restore orchestration
│   ├── model.go      # Bubble Tea state management
│   └── ui.go         # Beautiful interface rendering
├── bin/              # Compiled binaries
│   └── migrate       # Static binary executable
└── go.mod            # Go module definition
```

### Clean Architecture Benefits
- **Focused responsibilities** - Each file handles a specific domain
- **Easier maintenance** - Find and modify functionality quickly
- **Better testing** - Test individual components in isolation
- **Reduced complexity** - No more massive single files
- **Improved collaboration** - Multiple developers can work on different areas

### Pure Go Design
- **Zero CGO** - Built with `CGO_ENABLED=0`
- **Static linking** - Single binary deployment
- **Cross-platform** - Works on any Linux system
- **Memory safe** - No unsafe operations or C dependencies

## 🎨 Tokyo Night Theme

### Color Palette
- **Primary**: `#7aa2f7` (Beautiful blue)
- **Secondary**: `#9ece6a` (Success green)
- **Accent**: `#f7768e` (Warning red)
- **Text**: `#c0caf5` (Readable foreground)
- **Background**: `#1a1b26` (Deep background)

## 🔧 Development

### Build Commands
```bash
# Development build
go build -o bin/migrate .

# Production build (static binary)
CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/migrate .

# Test build
go test ./...
```

### Dependencies
```go
// Pure Go dependencies only
require (
    github.com/charmbracelet/bubbletea v0.24.2
    github.com/charmbracelet/lipgloss v0.9.1
)
```

## 🎯 Recent Achievements

### ✅ Critical Bug Fix (v1.0.4)
- **FIXED**: Directory contents not being copied when directory creation fails
- **FIXED**: Changed `os.Mkdir` to `os.MkdirAll` for better reliability
- **IMPROVED**: Better error logging and recovery - continues processing even if directory creation fails
- **ENHANCED**: More detailed logging for directory operations

### ✅ Code Reorganization (v1.0.3)
- Split monolithic backup.go (1,700+ lines) into focused modules
- Separated utilities, filesystem operations, drive management, and operations
- Improved maintainability and development experience
- Better code organization for future features and collaboration

### ✅ TUI-Only Interface (v1.0.1)
- Removed all CLI options - beautiful interface always
- Simplified user experience - just run `migrate` 
- No confusion about modes or options
- Consistent, delightful interaction every time

### ✅ Enhanced Operations (v1.0.1)
- Added comprehensive dependency checking on startup
- Improved progress screen with app branding and log file location
- Smart cancellation handling - Ctrl+C shows "Canceling..." status
- Better logging system using ~/.cache/migrate/ or /tmp fallback
- Proper cancellation cleanup ensures safe operation termination

### ✅ Pure Go Implementation (v1.0.1)
- Complete rewrite using pure Go instead of rsync
- Zero external dependencies during backup operations
- Efficient file copying with proper error handling
- Static binary deployment

### ✅ Perfect Progress Tracking
- Fixed progress calculation to account for existing data
- Real-time updates every 200ms
- Accurate time estimation based on copy rate
- Smart initial progress display

### ✅ Production Ready
- Extensively tested and working in production
- Handles large filesystems (1.8TB+) efficiently
- Robust error handling and recovery
- Beautiful, intuitive user interface

## 🤝 Contributing

This is a personal tool, but the clean architecture makes it easy to:
- Add new backup strategies
- Implement additional filesystems
- Enhance the user interface
- Add new features

## 📄 License

Created by Cypher Riot  
Personal system administration toolkit

🔗 **Links:**
- **GitHub**: [https://github.com/CyphrRiot/Migrate](https://github.com/CyphrRiot/Migrate)
- **X (Twitter)**: [https://x.com/CyphrRiot](https://x.com/CyphrRiot)

---

**🎉 Achievement Unlocked**: TUI-only pure Go backup tool with real-time progress tracking and zero external dependencies!
