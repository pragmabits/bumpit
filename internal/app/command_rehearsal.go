package app

import (
	"context"
	"errors"
	"io"

	"github.com/pragmabits/bumpit/internal/gitx"
	"github.com/spf13/cobra"
)

// rehearsalKey is the key of the rehearsal in the context of a command.
type rehearsalKey struct{}

// rehearsal is a series of bumpit commands run as the command line would run
// them, in the order bumpit prints them, that write nothing: each edits the files
// as those before it would have left them. It is the one thing that refuses what
// bumpit proposes, so that bumpit prints no command it would itself refuse when
// they are followed in that order. What a command reads to find the file it edits
// still comes from the disk, peekConfig's repository key and whether a --config
// declares modules among it: no command printed before another changes either.
type rehearsal struct {
	// files are what the commands would have written, by the file each would
	// write to, links resolved.
	files map[string][]byte
	// snapshots are the repositories the commands read, by the path each
	// names, so that they read each once.
	snapshots map[string]*repositorySnapshot
}

func newRehearsal() *rehearsal {
	return &rehearsal{files: map[string][]byte{}, snapshots: map[string]*repositorySnapshot{}}
}

// rehearsalOf is the rehearsal command runs in, nil when it runs for real.
func rehearsalOf(command *cobra.Command) *rehearsal {
	commandContext := command.Context()
	if commandContext == nil {
		return nil
	}
	found, _ := commandContext.Value(rehearsalKey{}).(*rehearsal)
	return found
}

// run runs the bumpit command with the words as the command line would, its
// flags, the config file it reads, its edit and the proof of the edit, and returns
// why it would be refused. A command that would be refused leaves the rehearsal
// as it was.
func (r *rehearsal) run(words []string) error {
	rootCommand := newRootCmd()
	rootCommand.SetArgs(words)
	rootCommand.SetOut(io.Discard)
	rootCommand.SetErr(io.Discard)
	return rootCommand.ExecuteContext(context.WithValue(context.Background(), rehearsalKey{}, r))
}

// text is what the file at target holds in the rehearsal: what a command before
// would have written, else what it holds on disk.
func (r *rehearsal) text(target string, onDisk []byte) []byte {
	if r == nil {
		return onDisk
	}
	if written, found := r.files[target]; found {
		return written
	}
	return onDisk
}

// snapshotAt is what the commands read of the repository at path, shared by the
// whole rehearsal. Out of a rehearsal, a command has one of its own.
func (r *rehearsal) snapshotAt(path string) *repositorySnapshot {
	if r == nil {
		return newRepositorySnapshot(gitx.New(path))
	}
	snapshot, found := r.snapshots[path]
	if !found {
		snapshot = newRepositorySnapshot(gitx.New(path))
		r.snapshots[path] = snapshot
	}
	return snapshot
}

// refusalReason is why a command would be refused, as a proposal says it: what is
// wrong of the module, when that is the reason, since nothing was edited.
func refusalReason(err error) string {
	if finding, isFinding := errors.AsType[Finding](err); isFinding {
		return finding.Error()
	}
	return err.Error()
}
