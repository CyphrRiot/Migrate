# Migrate

A stunningly beautiful **Terminal** backup and restore tool built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lipgloss](https://github.com/charmbracelet/lipgloss). Features Tokyo Night theming, pure Go implementation with zero external dependencies, and full `rsync --delete` equivalent functionality.

![Migrate Beautiful Interface](images/interface.gif)

## 🔥 **Live System Backup & Restore**

**The game changer:** Migrate backs up and restores **running live systems** - no downtime required.

Unlike traditional tools that require booting from external media (looking at you, CloneZilla), Migrate works seamlessly on your **active, running OS**. When you're ready to restore, simply:

1. **Fresh install** your OS 
2. **Run the restore** 
3. **💥 You're back up and running** - exactly as you left it

**No external boot disks. No system downtime. Just pure magic.** ✨

> ⚠️ **Testing Recommended**: While extensively tested, please test backup and restore operations in non-critical environments first.

## ✨ Features

- 🎨 **Beautiful TUI** - Tokyo Night color scheme with smooth animations  
- 🚀 **Pure Go** - Zero external dependencies, single static binary
- 💾 **System Backup** - Complete 1:1 backup or home directory only
- 🔄 **Smart Sync** - SHA256-based deduplication, automatic cleanup of deleted files
- 📊 **Real-time Progress** - File-based progress tracking with accurate estimates
- 🔒 **LUKS Support** - Works with encrypted external drives
- ⚡ **Fast** - rsync --delete equivalent performance in pure Go

## 🚀 Installation

### Download Binary (Recommended)
```bash
curl -L -o migrate https://github.com/CyphrRiot/Migrate/raw/main/bin/migrate
chmod +x migrate
mv migrate ~/.local/bin/  # optional
```

### Build From Source
```bash
git clone https://github.com/CyphrRiot/Migrate.git
cd Migrate
make build     # or: go build -o bin/migrate .
make install   # optional
```

## 🖥️ Usage

```bash
sudo migrate
```

That's it! The tool launches a beautiful TUI interface:
- 🎨 Navigate with arrow keys and Enter
- 🔍 Automatically detects external drives  
- 📊 Watch real-time progress with smooth animations

## ⚙️ How It Works

### rsync --delete Equivalent
Performs the equivalent of `rsync -aAx --delete / /backup/destination/` but with:
- **SHA256 verification** - Stronger integrity than timestamp comparison
- **Better logging** - Detailed statistics on copied vs. skipped files
- **Zero dependencies** - No rsync binary required
- **Beautiful interface** - Progress tracking and status updates

### Smart Features
- **File deduplication** - Skip identical files using SHA256 hashes
- **Incremental backups** - Only copy changed/new files
- **Automatic exclusions** - `/dev`, `/proc`, `/sys`, `/tmp`, backup destination
- **Progress accuracy** - File-based tracking: `(processed / total) * 85% + cleanup * 15%`

## 💾 Drive Support

Works with any external drive:
- **USB, SSD, HDD** - Automatic detection of removable drives
- **Multiple filesystems** - ext4, btrfs, exfat, NTFS
- **LUKS encryption** - Full encrypted drive support with helpful unlock instructions

## 🏗️ Architecture

```
├── main.go           # Entry point and TUI initialization
├── internal/         # Internal package
│   ├── version.go    # Version management
│   ├── utils.go      # Configuration and utilities
│   ├── filesystem.go # File operations
│   ├── drives.go     # Drive detection and mounting
│   ├── operations.go # Backup/restore logic
│   ├── model.go      # Bubble Tea state management
│   └── ui.go         # Interface rendering
└── bin/migrate       # Static binary
```

### Technical Details
- **Static binary** - Built with `CGO_ENABLED=0` for maximum portability
- **Memory efficient** - Direct filesystem operations using `filepath.WalkDir`
- **Error resilient** - Continues operation when individual files fail
- **Fast updates** - Progress refreshes every 200ms

## 🎨 Tokyo Night Theme

Beautiful color palette designed for readability:
- **Primary**: `#7aa2f7` (Blue) - **Secondary**: `#9ece6a` (Green)
- **Accent**: `#f7768e` (Red) - **Text**: `#c0caf5` (Foreground)

## 🔧 Development

```bash
# Development build
go build -o bin/migrate .

# Static binary (production)
CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/migrate .

# Dependencies (pure Go only)
# - github.com/charmbracelet/bubbletea
# - github.com/charmbracelet/lipgloss
```

## 🤝 Contributing

Clean architecture makes it easy to add new features. The tool is organized into focused modules for better maintainability.

## 📄 License

Created by **Cypher Riot**

🔗 **Links:**
- **GitHub**: https://github.com/CyphrRiot/Migrate
- **X**: https://x.com/CyphrRiot

---
*🎉 TUI-only pure Go backup tool with zero external dependencies!*
