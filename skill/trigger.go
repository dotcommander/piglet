package skill

import (
	"context"

	"github.com/dotcommander/piglet/ext"
)

func registerTrigger(app *ext.App, store *Store) {
	app.RegisterMessageHook(ext.MessageHook{
		Name:     "skill-trigger",
		Priority: 500,
		OnMessage: func(_ context.Context, msg string) (string, error) {
			matches := store.Match(msg)
			if len(matches) == 0 {
				return "", nil
			}
			// Load first match (most specific trigger)
			content, err := store.Load(matches[0].Name)
			if err != nil {
				return "", nil
			}
			return "# Skill: " + matches[0].Name + "\n\n" + content, nil
		},
	})
}
