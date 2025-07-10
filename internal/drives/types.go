// Package drives provides drive detection, mounting, and management functionality.
// This module defines the core types used throughout the drives package.
package drives

// DriveInfo contains metadata about an external drive available for backup operations.
// Represents both the physical device and its current mount status.
type DriveInfo struct {
	Device     string // Mount point path (e.g., "/run/media/user/drive") or device path
	Size       string // Human-readable size string (e.g., "1.5T", "500G")
	Label      string // Volume label or friendly name
	UUID       string // Filesystem UUID for identification
	Filesystem string // Filesystem type (e.g., "ext4", "ntfs", "crypto_LUKS")
	Encrypted  bool   // true if this is an encrypted drive (LUKS)
}

// DrivesLoaded is a Bubble Tea message containing the results of drive enumeration.
type DrivesLoaded struct {
	Drives []DriveInfo // List of discovered external drives (exported field)
}

// HomeFolderInfo represents metadata about a directory within the user's home folder.
// Used for selective backup operations to track folder size, visibility, and user selections.
// Now supports subfolder navigation for granular backup control.
type HomeFolderInfo struct {
	Name          string // Directory name (e.g., "Documents", ".config")
	Path          string // Full absolute path to the directory
	Size          int64  // Total size in bytes (calculated recursively)
	IsVisible     bool   // false for dotfiles/dotdirs (hidden folders)
	Selected      bool   // Current user selection state for backup inclusion
	AlwaysInclude bool   // true for dotdirs (automatically included, non-selectable)

	// NEW: Sub-folder support for granular selection
	HasSubfolders bool             // true if this folder contains discoverable subfolders
	Subfolders    []HomeFolderInfo // Only 1 level deep, populated on-demand
	ParentPath    string           // For breadcrumb navigation (empty for root level)
}

// LsblkDevice represents a block device from lsblk JSON output.
// Optimized structure to replace anonymous structs in LoadDrives().
type LsblkDevice struct {
	Name       string        `json:"name"`
	Size       string        `json:"size"`
	Label      string        `json:"label"`
	UUID       string        `json:"uuid"`
	Fstype     string        `json:"fstype"`
	Mountpoint string        `json:"mountpoint"`
	Type       string        `json:"type"`
	Hotplug    bool          `json:"hotplug"`
	Children   []LsblkDevice `json:"children"`
}

// LsblkOutput represents the root JSON structure from lsblk command.
type LsblkOutput struct {
	BlockDevices []LsblkDevice `json:"blockdevices"`
}

// MountedFilesystem represents a filesystem that's currently mounted.
// Used internally for drive detection optimization.
type MountedFilesystem struct {
	Device     string
	Size       string
	Label      string
	UUID       string
	Filesystem string
	Encrypted  bool
	MountPoint string
}
