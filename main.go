package main

import (
	"migrate/internal/app"
	"migrate/internal/platform"
)

func main() {
	platform.EnsureRoot()
	app.Run()
}
