# Migrate TODO Status

## ✅ COMPLETED: Restore Size Validation (COMPLETE FIX)

### 🚨 **CRITICAL BUG FIXED: Missing Restore Space Check & Incorrect Size Calculation**

**Issues**: 
1. The restore functionality was missing size validation entirely
2. When validation was added, it calculated the ENTIRE backup size, not just selected items

**Root Causes**: 
1. No space checking in restore workflows
2. `checkRestoreSpaceRequirements()` used `getUsedDiskSpace()` which counts ALL backup data, ignoring user selections

**Complete Solution Implemented**:

#### **1. ✅ Created Selective Restore Space Check Function**
- **New Function**: `checkSelectiveRestoreSpaceRequirements()` in `drives.go`
- **Logic**: Only counts folders/items the user actually selected to restore
- **Accuracy**: Includes config/window manager size estimates when selected
- **Result**: Precise space calculation based on user selections

#### **2. ✅ Updated Home Restore Space Check**  
- **Location**: `handleRestoreFolderSelection()` Continue button handler
- **Change**: Now calls `checkSelectiveRestoreSpaceRequirements()` instead of generic function
- **Timing**: Validates BEFORE showing confirmation dialog
- **Parameters**: Passes restore folders, selections, config flags

#### **3. ✅ System Restore Space Check (Original)**
- **Location**: `BackupDriveStatus` message handler after system backup detection
- **Function**: Still uses `checkRestoreSpaceRequirements()` (appropriate for full system restores)
- **Timing**: After drive mount and backup detection, BEFORE confirmation dialog

#### **4. ✅ Proper Error Flow & User Experience**
- **Error Display**: Uses `ScreenError` with manual dismissal
- **User Blocking**: Prevents confirmation dialog if insufficient space
- **Error Messages**: Detailed space breakdown with selected vs available space

**Technical Implementation**:

```go
// NEW: Selective restore space checking 
func checkSelectiveRestoreSpaceRequirements(
    restoreFolders []HomeFolderInfo, 
    selectedFolders map[string]bool, 
    restoreConfig bool, 
    restoreWindowMgrs bool,
) error {
    // Only count selected folders + config estimates
    var totalSelectedSize int64
    for _, folder := range restoreFolders {
        if folder.AlwaysInclude || selectedFolders[folder.Path] {
            totalSelectedSize += folder.Size
        }
    }
    // Add config estimates if selected
    if restoreConfig { totalSelectedSize += 100MB }
    if restoreWindowMgrs { totalSelectedSize += 50MB }
    
    // Compare against internal drive capacity
    return validateSpace(totalSelectedSize, internalTotalSize)
}
```

**Before vs After**:
- **Before**: 750GB backup → "not enough space" for 500GB drive (even if user only selected 100GB worth)
- **After**: User selects 100GB worth → space check passes ✅, 400GB worth → space error ❌

**Error Message Examples**:

**System Restore (Full Backup)**:
```
⚠️ INSUFFICIENT SPACE for restore

Backup size: 750GB
Internal drive total: 500GB

The backup is too large to fit on your internal drive.
You need at least 750GB of total drive capacity.
```

**Selective Restore (Only Selected Items)**:  
```
⚠️ INSUFFICIENT SPACE for restore

Selected items size: 600GB
Internal drive total: 500GB

The selected items are too large to fit on your internal drive.
You need at least 600GB of total drive capacity.
```

**Files Modified**:
- `internal/drives.go` - Added `checkSelectiveRestoreSpaceRequirements()` function
- `internal/model.go` - Updated restore selection to use selective space checking

**Critical Fix**: Now calculates space requirements based on **user selections**, not entire backup size. Restore space validation is accurate and prevents impossible operations while allowing valid selective restores.

**Status**: ✅ **COMPLETELY FIXED** - Restore now properly validates space for selected items only

---

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
