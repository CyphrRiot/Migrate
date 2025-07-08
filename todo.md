# Migrate System - Development Status

**NEVER run `go build`** - **ALWAYS run `make`**
**NEVER RUN `migrate`** - it hangs the system
**NEVER commit todo.md**

## Development Workflow

1. Make code changes (one function/issue at a time)
2. Ask "Continue?" and WAIT FOR CONFIRMATION
3. Once ACCEPTED:
    - Increment `internal/version.go` (e.g., 1.0.29 → 1.0.30)
    - Run `make`
    - Commit with descriptive message
    - Tag version and push to GitHub

## Current Status: WORKING BUILD ✅

- **Version**: Development (post-menu reordering)
- **Build Status**: ✅ Compiles successfully
- **Last Major Change**: Fixed verification errors screen (detailed error display working)
- **Code Quality**: POOR - Monolithic structure needs refactoring

## Recent Completions ✅

### UI/UX Improvements

- **Main Menu**: Reordered to Backup → Verify → Restore → About → Exit
- **Visual Hierarchy**: Info boxes (purple border), warnings (orange border), app structure (gray)
- **Verification Screen**: Added missing app border for consistency
- **Browser Cache**: Fixed verification exclusions (glob pattern bug)
- **Verification Errors**: Fixed detailed error screen display and navigation

### Critical Bug Fixes

- **Selective Backup**: Now respects folder selections properly
- **Space Errors**: Properly detected and displayed with details
- **Confirmation Dialogs**: Show source backup size
- **Verification Errors**: Scrollable, categorized error display
- **Tokyo Night Theme**: Fixed dark backgrounds

### Code Quality

- **Menu Duplication**: Added `mainMenuChoices` constant (eliminated 12+ duplicates)
- **Missing Fields**: Added `verificationErrors[]` and `errorScrollOffset` to Model struct

## Priority Issues (Fix Next) 🔥

### 1. CRITICAL: Code Quality - Monolithic Structure

- **Problem**: `internal/model.go` is 2000+ lines with massive duplication
- **Issues**: Giant switch statements, repeated logic, non-modular design
- **Impact**: Extremely difficult to maintain, error-prone
- **Solution**: Refactor into smaller, focused modules

### 2. CRITICAL: Navigation Back Button Bug

- **Problem**: Subfolder "Back" goes to main menu instead of parent folder
- **Expected**: Videos subfolder → Back → HOME folder selection
- **Actual**: Videos subfolder → Back → Main menu
- **File**: `internal/model.go` (escape key handling)
- **Impact**: Users lose navigation context

### 3. CRITICAL: Orange Background in Confirmation Dialogs

- **Problem**: Confirmation dialogs show orange background instead of Tokyo Night dark
- **Expected**: Dark background (#1a1b26) with orange border
- **File**: `internal/ui.go` (confirmation dialog styles)
- **Impact**: Breaks visual consistency

### 4. EMERGENCY: Selective Backup Cleanup Data Loss

- **Status**: HOTFIX APPLIED - selective cleanup disabled
- **Problem**: Path comparison fails between source and backup paths
- **Example**: `/home/grendel/Videos` vs `/run/media/grendel/Media/Videos`
- **Priority**: Must fix before re-enabling selective cleanup

## Future Improvements

### File Management

- **File Removal**: Implement rsync --delete behavior (remove files from backup that no longer exist in source)
- **Logging**: Reduce verbosity, overwrite logs on startup instead of appending

### User Experience

- **Root Protection**: Prevent home backup when running as root without sudo
- **Home Detection**: Unified function for proper home directory detection

## Restart Instructions

If starting fresh:

1. **Current State**: Working build with menu reordering and UI fixes
2. **Next Priority**: Fix navigation back button bug (issue #1 above)
3. **Build**: Always use `make`, never `go build`
4. **Test**: Test navigation from subfolder views before committing
5. **Code Quality**: Keep in mind the monolithic structure needs eventual refactoring

## File Structure Reference

- `internal/model.go` - Main application state (2000+ lines, needs refactoring)
- `internal/ui.go` - UI rendering and styles
- `internal/operations.go` - Backup/restore operations
- `internal/verification.go` - Verification logic
- `internal/filesystem.go` - File operations
- `internal/version.go` - Version tracking
