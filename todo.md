# Migrate TODO Status

## ✅ COMPLETED: Restore Folder Selection UI & Functionality

### UI Improvements (COMPLETED):
1. **✅ Configuration Options Styling**: 
   - "Restore Configuration" and "Window Managers" now styled in **teal color (Tokyo Night cyan)**
   - Visually distinct from regular folders using `tealGradient.Start` color
   - Different selection styling with teal background when selected

2. **✅ Visual Separator**:
   - **FIXED**: Separator line now appears AFTER both config options (below "Window Managers")
   - All folders now appear BELOW the separator line as intended
   - Uses `borderColor` styling for consistency with Tokyo Night theme

3. **✅ Restore Functionality Verified**:
   - ✅ **startSelectiveRestore()** properly respects folder selections
   - ✅ **restoreConfig** option controls `.config` directory restore
   - ✅ **restoreWindowMgrs** option controls window manager folders (Hyprland, GNOME, i3, etc.)
   - ✅ Only selected folders (`selectedFolders[folder.Path] = true`) are restored
   - ✅ Always-include folders (hidden dotfiles) are automatically restored
   - ✅ Comprehensive logging for verification

### 🚨 **CRITICAL RESTORE BEHAVIOR CLARIFICATION:**

**YES, files NOT present on the backup drive ARE deleted from the destination during restore!**

This implements **rsync --delete** equivalent behavior:

1. **Phase 1**: Copy/sync files from backup to destination
2. **Phase 2**: **DELETE extra files** that exist in destination but NOT in backup

**Code Evidence:**
```go
// Phase 2: Delete extra files for this folder
err = deleteExtraFiles(sourceFolderPath, targetFolderPath, logFile)
```

**deleteExtraFiles() function:**
- Walks the target directory 
- For each file/folder in target, checks if it exists in backup
- If NOT found in backup: **DELETES IT** (`os.Remove()` or `os.RemoveAll()`)

**This ensures the destination EXACTLY matches the backup** (like rsync --delete)

### Technical Implementation:
- **Config styling**: Teal color scheme distinguishes config options from folders
- **Separator placement**: Now correctly appears after ALL config options, before folders
- **Restore logic**: `performSelectiveRestore()` filters folders based on:
  - Folder selection state (`selectedFolders` map)
  - Config option (`restoreConfig` flag)
  - Window manager option (`restoreWindowMgrs` flag) 
  - Always-include flag for hidden folders
- **Deletion behavior**: Implements rsync --delete for exact backup/destination matching
- **Window manager detection**: Covers 20+ desktop environments and window managers

## Previous Issues (ALL RESOLVED):
- ✅ Fixed restore folder selection layout mismatch with backup
- ✅ Unified navigation logic between backup and restore
- ✅ Applied consistent styling and cursor behavior
- ✅ Distinguished configuration options from folders with colors
- ✅ **FIXED**: Separator line placement - now appears after BOTH config options
- ✅ Verified restore functionality works as designed
- ✅ **CLARIFIED: Restore deletes files not in backup (rsync --delete behavior)**

**Status: FEATURE COMPLETE ✅**
