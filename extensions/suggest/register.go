package suggest

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	sdk "github.com/dotcommander/piglet/sdk"
)

// Register wires the suggest extension into a shared SDK extension.
func Register(e *sdk.Extension) {
	var (
		s   *Suggester
		cwd atomic.Value
	)

	e.OnInit(func(x *sdk.Extension) {
		cwd.Store(x.CWD())
		cfg := LoadConfig()
		prompt := LoadPrompt()
		s = NewSuggester(cfg, prompt, x)
	})

	e.RegisterEventHandler(sdk.EventHandlerDef{
		Name:     "suggest-turn-end",
		Priority: 200,
		Events:   []string{"EventTurnEnd"},
		Handle: func(ctx context.Context, _ string, data json.RawMessage) *sdk.Action {
			if s == nil {
				return nil
			}

			if !s.ShouldSuggest() {
				return nil
			}

			var turn TurnData
			if err := json.Unmarshal(data, &turn); err != nil {
				return nil
			}

			if len(turn.ToolResults) == 0 {
				return nil
			}

			workingDir, _ := cwd.Load().(string)
			projCtx := GatherContext(workingDir, turn)

			suggestCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			suggestion, err := s.Generate(suggestCtx, turn, projCtx)
			if err != nil || suggestion == "" {
				return nil
			}

			s.ResetCooldown()
			return sdk.ActionShowMessage(suggestion)
		},
	})
}
