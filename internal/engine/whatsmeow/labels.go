package whatsmeow

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
	waEvents "go.mau.fi/whatsmeow/types/events"

	"wa-gateway/internal/engine"
)

// rememberLabel atualiza o cache local a partir de um evento LabelEdit
// (inclusive os do fullSync no reconnect).
func (e *Engine) rememberLabel(le *waEvents.LabelEdit) {
	if le == nil || le.LabelID == "" {
		return
	}
	e.labelsMu.Lock()
	defer e.labelsMu.Unlock()
	if e.labels == nil {
		e.labels = map[string]engine.Label{}
	}
	if le.Action != nil && le.Action.GetDeleted() {
		delete(e.labels, le.LabelID)
		return
	}
	l := e.labels[le.LabelID]
	l.ID = le.LabelID
	if le.Action != nil {
		if le.Action.Name != nil {
			l.Name = le.Action.GetName()
		}
		if le.Action.Color != nil {
			l.Color = le.Action.GetColor()
		}
	}
	e.labels[le.LabelID] = l
}

func (e *Engine) Labels(_ context.Context) ([]engine.Label, error) {
	e.labelsMu.RLock()
	defer e.labelsMu.RUnlock()
	out := make([]engine.Label, 0, len(e.labels))
	for _, l := range e.labels {
		out = append(out, l)
	}
	return out, nil
}

func (e *Engine) EditLabel(ctx context.Context, labelID, name string, color int32, deleted bool) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	if labelID == "" {
		return fmt.Errorf("labelId obrigatorio")
	}
	return client.SendAppState(ctx, appstate.BuildLabelEdit(labelID, name, color, deleted))
}

func (e *Engine) SetChatLabel(ctx context.Context, chatID, labelID string, on bool) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("chatId invalido: %w", err)
	}
	return client.SendAppState(ctx, appstate.BuildLabelChat(jid, labelID, on))
}

func (e *Engine) SetMessageLabel(ctx context.Context, chatID, messageID, labelID string, on bool) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	jid, err := types.ParseJID(chatID)
	if err != nil {
		return fmt.Errorf("chatId invalido: %w", err)
	}
	return client.SendAppState(ctx, appstate.BuildLabelMessage(jid, labelID, messageID, on))
}
