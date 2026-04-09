package shell

import (
	"github.com/dotcommander/piglet/ext"
	"github.com/dotcommander/piglet/session"
)

// DrainActions processes pending ext.App actions. Public for cases where
// frontends trigger actions outside the normal event flow (shortcuts, modal
// callbacks, async action results).
func (s *Shell) DrainActions() { s.drainActions() }

// drainActions processes pending ext.App actions, classifying each as
// internal (handled by Shell) or frontend-visible (surfaced as Notification).
func (s *Shell) drainActions() {
	if s.app == nil {
		return
	}

	for _, action := range s.app.PendingActions() {
		switch act := action.(type) {
		// --- Shell handles internally ---
		case ext.ActionSetSessionTitle:
			s.mu.Lock()
			sess := s.session
			s.mu.Unlock()
			if sess != nil && act.Title != "" {
				_ = sess.SetTitle(act.Title)
			}
			s.notify(Notification{Kind: NotifySessionTitle, Text: act.Title, Action: act})

		case ext.ActionRunAsync:
			// Run synchronously — frontends that need async can check Action
			// and handle it themselves via EnqueueResult.
			result := act.Fn()
			if result != nil {
				s.app.EnqueueAction(result)
			}

		case ext.ActionSendMessage:
			s.mu.Lock()
			running := s.running
			s.mu.Unlock()
			if running {
				s.enqueue(act.Content, false)
			} else {
				s.notify(Notification{Kind: NotifySendMessage, Text: act.Content, Action: act})
			}

		case ext.ActionSwapSession:
			if sess, ok := act.Session.(*session.Session); ok {
				s.mu.Lock()
				old := s.session
				s.session = sess
				agent := s.agent
				s.mu.Unlock()
				if old != nil && old != sess {
					_ = old.Close()
				}
				if agent != nil {
					agent.SetMessages(sess.Messages())
				}
				s.notify(Notification{Kind: NotifySessionSwap, Action: act})
			}

		case ext.ActionQuit:
			s.StopBackground()
			s.mu.Lock()
			s.quitting = true
			s.mu.Unlock()
			s.notify(Notification{Kind: NotifyQuit})

		// --- Surfaced to frontend ---
		case ext.ActionShowMessage:
			s.notify(Notification{Kind: NotifyMessage, Text: act.Text, Action: act})

		case ext.ActionNotify:
			kind := NotifyMessage
			switch act.Level {
			case "warn":
				kind = NotifyWarn
			case "error":
				kind = NotifyError
			}
			s.notify(Notification{Kind: kind, Text: act.Message, Action: act})

		case ext.ActionSetStatus:
			s.notify(Notification{Kind: NotifyStatus, Key: act.Key, Text: act.Text, Action: act})

		case ext.ActionShowPicker:
			s.notify(Notification{Kind: NotifyPicker, Action: act})

		case ext.ActionAttachImage:
			s.notify(Notification{Kind: NotifyImage, Action: act})

		case ext.ActionDetachImage:
			s.notify(Notification{Kind: NotifyImage, Action: act})

		case ext.ActionSetWidget:
			s.notify(Notification{Kind: NotifyWidget, Key: act.Key, Action: act})

		case ext.ActionShowOverlay:
			s.notify(Notification{Kind: NotifyOverlay, Key: act.Key, Action: act})

		case ext.ActionCloseOverlay:
			s.notify(Notification{Kind: NotifyOverlay, Key: act.Key, Action: act})

		case ext.ActionExec:
			s.notify(Notification{Kind: NotifyExec, Action: act})
		}
	}
}
