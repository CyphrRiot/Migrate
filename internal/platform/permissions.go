package platform

// PrivLevel represents the privilege level of the running process.
type PrivLevel int

const (
	PrivUser PrivLevel = iota // Running as regular user
	PrivRoot                  // Running as root
)
