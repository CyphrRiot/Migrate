# Makefile for Migrate

.PHONY: build clean install uninstall run test static

# Build the application (static binary)
build:
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/migrate .

# Build static binary (explicit target)
static:
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o bin/migrate .

# Clean build artifacts
clean:
	rm -f bin/migrate

# Install to ~/.local/bin
install: build
	mkdir -p ~/.local/bin
	cp bin/migrate ~/.local/bin/
	chmod +x ~/.local/bin/migrate

# Uninstall from ~/.local/bin
uninstall:
	rm -f ~/.local/bin/migrate

# Run the application
run: build
	bin/migrate

# Test build
test:
	CGO_ENABLED=0 go build -a -ldflags '-extldflags "-static"' -o /tmp/migrate-test .
	rm /tmp/migrate-test

# Development run (with go run)
dev:
	go run .
