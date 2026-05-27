package main

import (
	"migrate/internal/app"
	"migrate/internal/platform"
)

func main() {
	perm := platform.CheckPrivileges()
	app.Run(perm)
}
