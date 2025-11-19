package mode

// Mode represents the user's current operational mode
type Mode string

const (
	// ModePersonal is for individual productivity tracking
	ModePersonal Mode = "personal"
	
	// ModeTeam is for team collaboration and shared tracking
	ModeTeam Mode = "team"
)

// IsValid checks if a mode is valid
func (m Mode) IsValid() bool {
	switch m {
	case ModePersonal, ModeTeam:
		return true
	default:
		return false
	}
}

// String returns the string representation of a mode
func (m Mode) String() string {
	return string(m)
}

// DefaultMode returns the default mode for new users
func DefaultMode() Mode {
	return ModePersonal
}
