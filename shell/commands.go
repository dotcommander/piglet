package shell

import (
	"maps"
	"slices"

	"github.com/dotcommander/piglet/ext"
)

// Commands returns all registered slash commands.
func (s *Shell) Commands() map[string]*ext.Command {
	if s.app == nil {
		return nil
	}
	return s.app.Commands()
}

// CommandDesc holds a command name and its one-line description for autocomplete.
type CommandDesc struct {
	Name        string
	Description string
}

// CommandInfos returns a sorted list of registered commands with their descriptions.
func (s *Shell) CommandInfos() []CommandDesc {
	if s.app == nil {
		return nil
	}
	cmds := s.app.Commands()
	names := slices.Sorted(maps.Keys(cmds))
	out := make([]CommandDesc, 0, len(names))
	for _, name := range names {
		out = append(out, CommandDesc{Name: name, Description: cmds[name].Description})
	}
	return out
}
