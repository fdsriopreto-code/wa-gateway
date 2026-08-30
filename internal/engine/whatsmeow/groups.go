package whatsmeow

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"wa-gateway/internal/engine"
)

// --- consultas ---

func (e *Engine) CheckOnWhatsApp(ctx context.Context, phones []string) ([]engine.OnWhatsApp, error) {
	client, err := e.currentClient()
	if err != nil {
		return nil, err
	}
	resp, err := client.IsOnWhatsApp(ctx, phones)
	if err != nil {
		return nil, err
	}
	out := make([]engine.OnWhatsApp, 0, len(resp))
	for _, r := range resp {
		item := engine.OnWhatsApp{Query: r.Query, JID: r.JID.String(), IsRegistered: r.IsIn}
		if r.VerifiedName != nil && r.VerifiedName.Details != nil {
			item.VerifiedName = r.VerifiedName.Details.GetVerifiedName()
		}
		out = append(out, item)
	}
	return out, nil
}

func (e *Engine) GetUserInfo(ctx context.Context, jids []string) ([]engine.UserInfo, error) {
	client, err := e.currentClient()
	if err != nil {
		return nil, err
	}
	parsed := make([]types.JID, 0, len(jids))
	for _, s := range jids {
		j, err := types.ParseJID(s)
		if err != nil {
			return nil, fmt.Errorf("jid invalido %q: %w", s, err)
		}
		parsed = append(parsed, j)
	}
	info, err := client.GetUserInfo(ctx, parsed)
	if err != nil {
		return nil, err
	}
	out := make([]engine.UserInfo, 0, len(info))
	for jid, u := range info {
		item := engine.UserInfo{JID: jid.String(), Status: u.Status, PictureID: u.PictureID}
		if u.VerifiedName != nil && u.VerifiedName.Details != nil {
			item.VerifiedName = u.VerifiedName.Details.GetVerifiedName()
		}
		for _, d := range u.Devices {
			item.Devices = append(item.Devices, d.String())
		}
		out = append(out, item)
	}
	return out, nil
}

func (e *Engine) GetProfilePicture(ctx context.Context, jid string, preview bool) (string, error) {
	client, err := e.currentClient()
	if err != nil {
		return "", err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("jid invalido: %w", err)
	}
	info, err := client.GetProfilePictureInfo(ctx, j, &whatsmeow.GetProfilePictureParams{Preview: preview})
	if err != nil {
		return "", err
	}
	if info == nil {
		return "", nil
	}
	return info.URL, nil
}

// Contacts devolve a agenda inteira da sessao a partir do contact store local
// do whatsmeow (populado por sync inicial + push names vistos em conversas).
func (e *Engine) Contacts(ctx context.Context) ([]engine.ContactEntry, error) {
	client, err := e.currentClient()
	if err != nil {
		return nil, err
	}
	all, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]engine.ContactEntry, 0, len(all))
	for jid, c := range all {
		entry := engine.ContactEntry{
			JID:          jid.ToNonAD().String(),
			FirstName:    c.FirstName,
			FullName:     c.FullName,
			PushName:     c.PushName,
			BusinessName: c.BusinessName,
		}
		if jid.Server == types.DefaultUserServer {
			entry.Phone = jid.User
		}
		out = append(out, entry)
	}
	return out, nil
}

// --- grupos ---

func toGroup(gi *types.GroupInfo) engine.Group {
	g := engine.Group{
		JID:      gi.JID.String(),
		Name:     gi.Name,
		Topic:    gi.Topic,
		Owner:    gi.OwnerJID.String(),
		Announce: gi.IsAnnounce,
		Locked:   gi.IsLocked,
	}
	if !gi.GroupCreated.IsZero() {
		g.Created = gi.GroupCreated.Unix()
	}
	for _, p := range gi.Participants {
		g.Participants = append(g.Participants, engine.GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		})
	}
	return g
}

func (e *Engine) ListGroups(ctx context.Context) ([]engine.Group, error) {
	client, err := e.currentClient()
	if err != nil {
		return nil, err
	}
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]engine.Group, 0, len(groups))
	for _, gi := range groups {
		out = append(out, toGroup(gi))
	}
	return out, nil
}

func (e *Engine) GroupInfo(ctx context.Context, jid string) (engine.Group, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.Group{}, err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return engine.Group{}, fmt.Errorf("jid invalido: %w", err)
	}
	gi, err := client.GetGroupInfo(ctx, j)
	if err != nil {
		return engine.Group{}, err
	}
	return toGroup(gi), nil
}

func (e *Engine) CreateGroup(ctx context.Context, name string, participants []string) (engine.Group, error) {
	client, err := e.currentClient()
	if err != nil {
		return engine.Group{}, err
	}
	jids, err := parseJIDs(participants)
	if err != nil {
		return engine.Group{}, err
	}
	gi, err := client.CreateGroup(ctx, whatsmeow.ReqCreateGroup{Name: name, Participants: jids})
	if err != nil {
		return engine.Group{}, err
	}
	return toGroup(gi), nil
}

func (e *Engine) LeaveGroup(ctx context.Context, jid string) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("jid invalido: %w", err)
	}
	return client.LeaveGroup(ctx, j)
}

func (e *Engine) UpdateParticipants(ctx context.Context, jid string, action engine.ParticipantAction, participants []string) ([]engine.GroupParticipant, error) {
	client, err := e.currentClient()
	if err != nil {
		return nil, err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("jid invalido: %w", err)
	}
	jids, err := parseJIDs(participants)
	if err != nil {
		return nil, err
	}
	var pc whatsmeow.ParticipantChange
	switch action {
	case engine.ParticipantAdd:
		pc = whatsmeow.ParticipantChangeAdd
	case engine.ParticipantRemove:
		pc = whatsmeow.ParticipantChangeRemove
	case engine.ParticipantPromote:
		pc = whatsmeow.ParticipantChangePromote
	case engine.ParticipantDemote:
		pc = whatsmeow.ParticipantChangeDemote
	default:
		return nil, fmt.Errorf("acao invalida: %s", action)
	}
	res, err := client.UpdateGroupParticipants(ctx, j, jids, pc)
	if err != nil {
		return nil, err
	}
	out := make([]engine.GroupParticipant, 0, len(res))
	for _, p := range res {
		out = append(out, engine.GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		})
	}
	return out, nil
}

func (e *Engine) SetGroupName(ctx context.Context, jid, name string) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("jid invalido: %w", err)
	}
	return client.SetGroupName(ctx, j, name)
}

func (e *Engine) SetGroupTopic(ctx context.Context, jid, topic string) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("jid invalido: %w", err)
	}
	return client.SetGroupTopic(ctx, j, "", "", topic)
}

func (e *Engine) SetGroupPhoto(ctx context.Context, jid string, data []byte) (string, error) {
	client, err := e.currentClient()
	if err != nil {
		return "", err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("jid invalido: %w", err)
	}
	return client.SetGroupPhoto(ctx, j, data)
}

// SetGroupAnnounce: true = so admins enviam mensagens.
func (e *Engine) SetGroupAnnounce(ctx context.Context, jid string, on bool) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("jid invalido: %w", err)
	}
	return client.SetGroupAnnounce(ctx, j, on)
}

// SetGroupLocked: true = so admins editam nome/foto/descricao.
func (e *Engine) SetGroupLocked(ctx context.Context, jid string, on bool) error {
	client, err := e.currentClient()
	if err != nil {
		return err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("jid invalido: %w", err)
	}
	return client.SetGroupLocked(ctx, j, on)
}

func (e *Engine) GroupInviteLink(ctx context.Context, jid string, reset bool) (string, error) {
	client, err := e.currentClient()
	if err != nil {
		return "", err
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("jid invalido: %w", err)
	}
	return client.GetGroupInviteLink(ctx, j, reset)
}

func (e *Engine) JoinGroupWithLink(ctx context.Context, code string) (string, error) {
	client, err := e.currentClient()
	if err != nil {
		return "", err
	}
	jid, err := client.JoinGroupWithLink(ctx, code)
	if err != nil {
		return "", err
	}
	return jid.String(), nil
}

func parseJIDs(in []string) ([]types.JID, error) {
	out := make([]types.JID, 0, len(in))
	for _, s := range in {
		j, err := types.ParseJID(s)
		if err != nil {
			return nil, fmt.Errorf("jid invalido %q: %w", s, err)
		}
		out = append(out, j)
	}
	return out, nil
}
