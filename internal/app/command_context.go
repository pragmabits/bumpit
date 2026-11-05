package app

import "strings"

// commandContext is what a command bumpit prints needs to reach what the run
// that prints it read, which depends on what the run received and on what its
// config made of that.
type commandContext struct {
	// repositoryGiven is the --repo of the command line, empty when it gives
	// none.
	repositoryGiven string
	// configGiven is the --config of the command line, empty when it gives
	// none, and configHoldsModules reports that the runs read the modules from
	// it.
	configGiven        string
	configHoldsModules bool
	// effective is the repository the run reads, the one the config gives when
	// the command line gives none.
	effective string
}

// repositoryFlags carry the --repo the run received.
func (c commandContext) repositoryFlags() []string {
	if c.repositoryGiven == "" {
		return nil
	}
	return []string{"-r", notAFlag(c.repositoryGiven)}
}

// moduleFlags are what a command that edits the modules needs to edit the file
// the run read them from, and to read the same paths: a --config silent on the
// modules leaves them to the top, and only its repository to the command.
func (c commandContext) moduleFlags() []string {
	switch {
	case c.configGiven != "" && c.configHoldsModules:
		return append([]string{"-c", notAFlag(c.configGiven)}, c.repositoryFlags()...)
	case c.configGiven != "":
		return []string{"-r", notAFlag(c.effective)}
	default:
		return c.repositoryFlags()
	}
}

// keyFlags are what bumpit config needs to find the file the run read its keys
// from: it looks in --repo, and applies no repository a config gives.
func (c commandContext) keyFlags() []string {
	if c.configGiven != "" {
		return []string{"-c", notAFlag(c.configGiven)}
	}
	return c.repositoryFlags()
}

// notAFlag is a path written after ./ when it starts with -, which a command
// would read as a flag.
func notAFlag(entry string) string {
	if strings.HasPrefix(entry, "-") {
		return "./" + entry
	}
	return entry
}
