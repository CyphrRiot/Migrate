# Makefile for Migrate
#
# IMPORTANT: NEVER run `go build` directly - it creates a `migrate` binary in root!
# ALWAYS use `make` which creates `bin/migrate` properly.

.PHONY: build clean install uninstall run test static

# Build the application (static binary) - NEVER use `go build` directly!
build:
	@mkdir -p bin
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/migrate .

# Build static binary (explicit target) - NEVER use `go build` directly!
static:
	@mkdir -p bin
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/migrate .

# Clean build artifacts (including any stray root binary)
clean:
	rm -f bin/migrate
	rm -f migrate

# Safety check for root binary (in case someone used `go build` by mistake)
check:
	@if [ -f migrate ]; then \
		echo "ERROR: Found 'migrate' binary in root directory!"; \
		echo "This should NOT exist. Only 'bin/migrate' should be created."; \
		echo "Someone probably ran 'go build' instead of 'make'."; \
		echo "Run 'make clean' to remove it."; \
		exit 1; \
	fi
	@echo "✅ No stray binaries found"

# Install to ~/.local/bin
install: build check
	mkdir -p ~/.local/bin
	cp bin/migrate ~/.local/bin/
	chmod +x ~/.local/bin/migrate

# Uninstall from ~/.local/bin
uninstall:
	rm -f ~/.local/bin/migrate

# Run the application
run: build check
	bin/migrate

# Test build
test:
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o /tmp/migrate-test .
	rm /tmp/migrate-test

# Development run (with go run)
dev:
	go run .
