# Terminal Compatibility Guide for Migrate

This guide helps ensure Migrate displays correctly across different terminals, fonts, and systems.

## Supported Terminals

Migrate has been tested and optimized for:

- **Linux**: GNOME Terminal, Konsole, Alacritty, Kitty, Terminator, xterm
- **macOS**: Terminal.app, iTerm2, Alacritty, Kitty
- **Windows**: Windows Terminal, PowerShell, Git Bash, WSL terminals
- **SSH**: Most SSH clients with UTF-8 support

## Font Requirements

For the best experience, use a monospace font that supports:

1. **Basic Box Drawing Characters**: ─ │ ╭ ╮ ╰ ╯
2. **Unicode Symbols**: ✓ ✗ ⚠️ 🔍 📁 💾
3. **Emoji** (optional but recommended): 🎉 ✨ 🏠

### Recommended Fonts

- **Nerd Fonts**: Any Nerd Font variant (FiraCode Nerd Font, JetBrains Mono Nerd Font)
- **Standard Fonts**: Consolas, Monaco, DejaVu Sans Mono, Ubuntu Mono
- **Modern Fonts**: Cascadia Code, JetBrains Mono, Fira Code

## Troubleshooting Display Issues

### Problem: Borders appear broken or misaligned

**Solution**: Ensure your terminal is using a monospace font. Variable-width fonts will break the UI layout.

```bash
# Check your terminal's font settings
# Most terminals: Edit → Preferences → Font
```

### Problem: Unicode symbols show as boxes or question marks

**Solution 1**: Switch to ASCII mode
```bash
export MIGRATE_ASCII=1
migrate
```

**Solution 2**: Install a font with better Unicode support
```bash
# Ubuntu/Debian
sudo apt install fonts-noto-color-emoji fonts-firacode

# Fedora
sudo dnf install google-noto-emoji-fonts fira-code-fonts

# macOS (with Homebrew)
brew tap homebrew/cask-fonts
brew install --cask font-fira-code
```

### Problem: Content appears cut off or overlaps

**Solution**: Migrate enforces safe terminal dimensions. If issues persist:

1. Resize your terminal to at least 80×24 characters
2. Check for unusual DPI or scaling settings
3. Try a different terminal emulator

### Problem: Colors don't display correctly

**Solution**: Ensure your terminal supports 256 colors or true color

```bash
# Check color support
echo $TERM
# Should show something like: xterm-256color, screen-256color, etc.

# If not, set it:
export TERM=xterm-256color
```

## Environment Variables

Migrate respects these environment variables for compatibility:

- `MIGRATE_ASCII=1` - Force ASCII-only mode (no Unicode)
- `NO_COLOR=1` - Disable all colors (follows no-color.org standard)
- `TERM` - Terminal type detection

## SSH and Remote Sessions

When using Migrate over SSH:

1. **Ensure UTF-8 locale**:
   ```bash
   locale | grep UTF
   # Should show UTF-8 entries

   # If not, set it:
   export LANG=en_US.UTF-8
   export LC_ALL=en_US.UTF-8
   ```

2. **Forward locale in SSH**:
   ```bash
   # In ~/.ssh/config
   Host *
       SendEnv LANG LC_*
   ```

3. **Use ASCII mode if needed**:
   ```bash
   MIGRATE_ASCII=1 migrate
   ```

## Terminal Size Detection

Migrate automatically detects terminal size and adjusts:

- **Minimum**: 80×24 characters (enforced)
- **Maximum**: 200×60 characters (capped for readability)
- **Recommended**: 120×30 or larger

## Known Issues

### Windows Console (cmd.exe)
- Limited Unicode support
- Automatically switches to ASCII mode
- Consider using Windows Terminal instead

### macOS Terminal.app
- Some emoji may render with incorrect width
- Box drawing characters work well
- Consider using iTerm2 for better emoji support

### Linux Console (TTY)
- No Unicode support
- Automatically uses ASCII mode
- Colors may be limited

## Testing Your Terminal

Run this command to test Unicode support:
```bash
echo "Box: ╭─╮ │ ╰─╯"
echo "Symbols: ✓ ✗ ⚠️ 🔍 📁 💾"
echo "Progress: ⣾⣽⣻⢿⡿⣟⣯⣷"
```

If any characters don't display correctly, use `MIGRATE_ASCII=1`.

## Accessibility

For screen readers and accessibility tools:

1. Use ASCII mode for better compatibility
2. Migrate uses semantic text that works without symbols
3. All UI elements have text descriptions

```bash
# For screen reader users
MIGRATE_ASCII=1 migrate
```

## Getting Help

If you experience display issues:

1. Take a screenshot showing the problem
2. Include your terminal and font information:
   ```bash
   echo "Terminal: $TERM"
   echo "Lang: $LANG"
   echo "Font: Check terminal preferences"
   ```
3. Report at: https://github.com/YourRepo/Migrate/issues
