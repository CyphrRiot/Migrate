# Makefile for Migrate

.PHONY: build clean install uninstall run test

# Build the application
build:
	go build -o migrate .

# Clean build artifacts
clean:
	rm -f migrate

# Install to ~/.local/bin
install: build
	mkdir -p ~/.local/bin
	cp migrate ~/.local/bin/
	chmod +x ~/.local/bin/migrate

# Uninstall from ~/.local/bin
uninstall:
	rm -f ~/.local/bin/migrate

# Run the application
run: build
	./migrate

# Test build
test:
	go build -o /tmp/migrate-test .
	rm /tmp/migrate-test

# Development run (with go run)
dev:
	go run .
