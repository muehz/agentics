package argv

// Word is a parsed command word with its quote and expansion provenance.
type Word struct {
	Text    string
	Raw     string
	Quoted  bool
	Literal bool
	// Substituted distinguishes command output from other runtime expansion.
	Substituted bool
}
