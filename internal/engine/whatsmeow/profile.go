package whatsmeow

import (
	"context"
	"fmt"

	waEvents "go.mau.fi/whatsmeow/types/events"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"wa-gateway/internal/engine"
)

// PairPhone pede um codigo de pareamento por telefone (sem QR). A sessao
// precisa estar iniciada e ainda nao pareada.
func (e *Engine) PairPhone(ctx context.Context, phone string) (string, error) {
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()
	if client == nil {
		return "", fmt.Errorf("sessao nao iniciada")
	}
	if client.Store.ID != nil {
		return "", fmt.Errorf("sessao ja esta pareada")
	}
	code, err := client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "wa-gateway")
	if err != nil {
		return "", err
	}
	return code, nil
}

func (e *Engine) Me(ctx context.Context) (engine.Me, error) {
	e.mu.RLock()
	client := e.client
	e.mu.RUnlock()
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return engine.Me{}, fmt.Errorf("sessao nao conectada")
	}
	st := client.Store
	me := engine.Me{
		JID:          st.ID.String(),
		PushName:     st.PushName,
		Platform:     st.Platform,
		BusinessName: st.BusinessName,
	}
	if !st.LID.IsEmpty() {
		me.LID = st.LID.String()
	}
	if devs, err := client.GetUserDevices(ctx, []types.JID{st.ID.ToNonAD()}); err == nil {
		for _, d := range devs {
			me.Devices = append(me.Devices, d.String())
		}
	}
	return me, nil
}

func (e *Engine) SetStatusMessage(ctx context.Context, text string) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	return client.SetStatusMessage(ctx, types.SetStatusInput{Text: proto.String(text)})
}

func (e *Engine) SetPresence(ctx context.Context, available bool) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	p := types.PresenceUnavailable
	if available {
		p = types.PresenceAvailable
	}
	return client.SendPresence(ctx, p)
}

func (e *Engine) SetBlocked(ctx context.Context, jid string, block bool) ([]string, error) {
	client, err := e.currentClient()
	if err != nil {
		return nil, err
	}
	j, err := types.ParseJID(normJID(jid))
	if err != nil {
		return nil, fmt.Errorf("jid invalido: %w", err)
	}
	action := waEvents.BlocklistChangeActionUnblock
	if block {
		action = waEvents.BlocklistChangeActionBlock
	}
	bl, err := client.UpdateBlocklist(ctx, j, action)
	if err != nil {
		return nil, err
	}
	return jidsToStrings(bl), nil
}

func (e *Engine) Blocklist(ctx context.Context) ([]string, error) {
	client, err := e.currentClient()
	if err != nil {
		return nil, err
	}
	bl, err := client.GetBlocklist(ctx)
	if err != nil {
		return nil, err
	}
	return jidsToStrings(bl), nil
}

func jidsToStrings(bl *types.Blocklist) []string {
	if bl == nil {
		return []string{}
	}
	out := make([]string, 0, len(bl.JIDs))
	for _, j := range bl.JIDs {
		out = append(out, j.String())
	}
	return out
}
