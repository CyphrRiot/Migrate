# Migrate System - Development Status

**NEVER run `go build`** - **ALWAYS run `make`**
**NEVER RUN `migrate`** - it hangs the system
**NEVER compile without user approval** - **ALWAYS wait for "Continue?" confirmation**

## 🚨 CRITICAL VERSION MANAGEMENT RULES 🚨

**DO NOT UPDATE VERSION UNTIL:**

1. **Code changes are FULLY TESTED and VERIFIED working**
2. **User has CONFIRMED the fix works**
3. **User has EXPLICITLY APPROVED version increment**

**VERSION SPAM PREVENTION:**

- No version increments for untested changes
- No version increments for failed attempts
- No tags until user confirms "this is a release"
- Only increment version when user says "this works, release it"

## Development Workflow

1. Make code changes (one function/issue at a time)
2. Show proposed changes and ask "Continue?"
3. **WAIT FOR USER APPROVAL** - Do NOT compile or commit without explicit "yes" or "continue"
4. **TEST THE CHANGES** - User must verify it works
5. Once TESTED AND CONFIRMED WORKING:
    - User approves version increment
    - Increment `internal/version.go` (e.g., 1.0.29 → 1.0.30)
    - Run `make`
    - Commit with descriptive message
    - Tag version and push to GitHub

## Priority Issues (Fix Next) 🔥

### 1. CRITICAL: Verification Hanging on Critical Files 🚨

- **Problem**: Verification hangs indefinitely on "Checking Critical Files \* 5 Verified"
- **Symptoms**: Progress stops after verifying 5 critical files, never completes
- **Likely Cause**: Stuck on 6th critical file - possibly `/boot/loader/loader.conf` or large `/boot/grub/grub.cfg`
- **Potential Issues**:
    - SHA256 calculation hanging on large files
    - Permission denied on protected files (e.g., `/etc/shadow`)
    - Missing timeout in `getFileSHA256` function
    - Infinite loop in file reading
- **Critical Files List**:
    1. `/etc/fstab`
    2. `/etc/passwd`
    3. `/etc/shadow`
    4. `/etc/group`
    5. `/boot/grub/grub.cfg` (likely culprit - can be large)
    6. `/boot/loader/loader.conf`
- **Impact**: Verification never completes, appears frozen to users
- **Priority**: 🚨 URGENT - Makes verification unusable
- **Next Steps**: Test if timeout fixes in v1.0.46 resolved this

### 2. CRITICAL: Verification Errors Display and Scrolling ✅ FIXED v1.0.47

- **Problem**: When more than 12 verification errors, scrolling didn't work
- **Symptoms**: Display showed all errors at once, going off screen
- **Root Cause**: Missing early returns in key handlers caused fall-through to cursor logic
- **Solution**: Added early returns in up/down key handlers for verification errors screen
- **Status**: ✅ FIXED - Scrolling now works with arrow keys, Page Up/Down, Home/End
- **Commit**: v1.0.47 - Fixed verification errors scrolling and display

### 3. CRITICAL: Backup/Verification Function Consolidation

- **Problem**: HOME and SYSTEM backup/verification use different patterns and logic
- **Problem**: Exclusion logic scattered across operations.go, verification.go, and utils.go
- **Problem**: System verification has hardcoded exclusions instead of using shared functions
- **Impact**: Inconsistent behavior between backup types, maintenance nightmare
- **Solution**: Create unified backup/verification framework that shares functions
- **Priority**: HIGH - This causes user confusion and bugs

### 4. CRITICAL: Navigation Back Button Bug

- **Problem**: Subfolder "Back" goes to main menu instead of parent folder
- **Expected**: Videos subfolder → Back → HOME folder selection
- **Actual**: Videos subfolder → Back → Main menu
- **File**: `internal/model.go` (escape key handling)
- **Impact**: Users lose navigation context

### 5. CRITICAL: Orange Background in Confirmation Dialogs

- **Problem**: Confirmation dialogs show orange background instead of Tokyo Night dark
- **Expected**: Dark background (#1a1b26) with orange border
- **File**: `internal/ui.go` (confirmation dialog styles)
- **Impact**: Breaks visual consistency
- **Status**: Attempted fix in v1.0.41, needs verification

### 6. EMERGENCY: Selective Backup Cleanup Data Loss

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

### Code Quality Goals

- **Modular Functions**: Extract common patterns into reusable functions
- **No Copy-Paste**: Same logic should exist in ONE place only
- **Configurable Patterns**: Browser cache exclusions in config, not hardcoded
- **Shared Libraries**: Common operations (file matching, exclusions) should be utilities

## Restart Instructions

If starting fresh:

1. **Current State**: Build compiles with critical fixes applied for verification system
2. **URGENT Priority**: Test verification to confirm Go cache files are now excluded
3. **Build**: Always use `make`, never `go build`
4. **Test**: Verification should now properly exclude cache files - **NEEDS TESTING**
5. **Code Quality**: Fixed three root causes - log path, pattern matching, system backup exclusions
6. **Version Management**: **ALL TAGS REMOVED** - No version increment until user confirms fix works
7. **Next Release**: Only when user confirms "this works, release it"
8. **Expected Result**: System backup verification should work like home backup verification

## Next Steps

1. ✅ COMPLETED: Verification errors scrolling fixed in v1.0.47
2. Next Priority: Backup/Verification Function Consolidation
3. Fix navigation back button bug
4. Fix orange background in confirmation dialogs
5. Continue refactoring other large files (operations.go, verification.go)
