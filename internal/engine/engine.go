// Package engine define o contrato que qualquer camada de WhatsApp deve
// cumprir (hoje: whatsmeow) e um registry de factories.
package engine

import (
	"context"
	"log/slog"
	"sync"

	"wa-gateway/internal/events"
)

type Status string

const (
	StatusStopped   Status = "STOPPED"
	StatusStarting  Status = "STARTING"
	StatusScanQR    Status = "SCAN_QR_CODE"
	StatusWorking   Status = "WORKING"
	StatusFailed    Status = "FAILED"
	StatusLoggedOut Status = "LOGGED_OUT"
)

// SendResult e o retorno minimo de um envio.
type SendResult struct {
	MessageID string `json:"messageId"`
	Timestamp int64  `json:"timestamp"`
}

// Media e um anexo a enviar. Filename so vale para documento; Seconds e
// Waveform sao dicas opcionais para audio/PTT.
type Media struct {
	Data     []byte
	Mimetype string
	Filename string
	Caption  string
	Seconds  uint32
	GIF      bool // trata o video como GIF playback
	Voice    bool // audio como nota de voz (PTT)
}

// Location e um ponto geografico para SendLocation.
type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name,omitempty"`
	Address   string  `json:"address,omitempty"`
}

// Contact e um cartao de contato para SendContact. Se VCard vier preenchido
// ele e usado como esta; senao a engine monta um a partir de Name/Phone.
type Contact struct {
	Name  string `json:"name"`
	Phone string `json:"phone,omitempty"`
	VCard string `json:"vcard,omitempty"`
}

// MessageRef aponta uma mensagem ja existente (para reagir, apagar, editar,
// marcar como lida). SenderID so e obrigatorio em grupo quando FromMe e false.
type MessageRef struct {
	ChatID   string `json:"chatId"`
	ID       string `json:"id"`
	FromMe   bool   `json:"fromMe"`
	SenderID string `json:"senderId,omitempty"`
}

// ChatState e o estado de presenca dentro de um chat.
type ChatState string

const (
	ChatStateTyping    ChatState = "typing"
	ChatStateRecording ChatState = "recording"
	ChatStatePaused    ChatState = "paused"
)

// OnWhatsApp e o resultado da checagem de um numero.
type OnWhatsApp struct {
	Query        string `json:"query"`
	JID          string `json:"jid"`
	IsRegistered bool   `json:"isRegistered"`
	VerifiedName string `json:"verifiedName,omitempty"`
}

// UserInfo e o perfil publico de um contato.
type UserInfo struct {
	JID          string   `json:"jid"`
	Status       string   `json:"status,omitempty"`
	PictureID    string   `json:"pictureId,omitempty"`
	VerifiedName string   `json:"verifiedName,omitempty"`
	Devices      []string `json:"devices,omitempty"`
}

// GroupParticipant e um membro de grupo.
type GroupParticipant struct {
	JID          string `json:"jid"`
	IsAdmin      bool   `json:"isAdmin"`
	IsSuperAdmin bool   `json:"isSuperAdmin"`
}

// Group e o resumo de um grupo.
type Group struct {
	JID          string             `json:"jid"`
	Name         string             `json:"name"`
	Topic        string             `json:"topic,omitempty"`
	Owner        string             `json:"owner,omitempty"`
	Created      int64              `json:"created,omitempty"`
	Announce     bool               `json:"announce"`
	Locked       bool               `json:"locked"`
	Participants []GroupParticipant `json:"participants,omitempty"`
}

// ParticipantAction e a operacao aplicada a membros de grupo.
type ParticipantAction string

const (
	ParticipantAdd     ParticipantAction = "add"
	ParticipantRemove  ParticipantAction = "remove"
	ParticipantPromote ParticipantAction = "promote"
	ParticipantDemote  ParticipantAction = "demote"
)

// Engine e uma sessao de WhatsApp viva. Implementacoes NAO devem bloquear
// em Start; o trabalho de conexao roda em background e o progresso e
// reportado via eventos (session.status / session.qr).
type Engine interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
	Logout(ctx context.Context) error

	Status() Status
	QR() string  // codigo QR atual, se em SCAN_QR_CODE
	JID() string // JID completo, se logado

	// --- envio ---
	SendText(ctx context.Context, chatID, text string) (SendResult, error)
	SendImage(ctx context.Context, chatID string, data []byte, mimetype, caption string) (SendResult, error)
	SendFile(ctx context.Context, chatID string, m Media) (SendResult, error)
	SendVideo(ctx context.Context, chatID string, m Media) (SendResult, error)
	SendAudio(ctx context.Context, chatID string, m Media) (SendResult, error)
	SendLocation(ctx context.Context, chatID string, loc Location) (SendResult, error)
	SendContact(ctx context.Context, chatID string, cs []Contact) (SendResult, error)

	// --- operacoes sobre mensagens ---
	SendReaction(ctx context.Context, ref MessageRef, emoji string) (SendResult, error)
	DeleteMessage(ctx context.Context, ref MessageRef) (SendResult, error)
	EditMessage(ctx context.Context, ref MessageRef, newText string) (SendResult, error)
	MarkRead(ctx context.Context, ref MessageRef) error
	SendChatPresence(ctx context.Context, chatID string, state ChatState) error

	// --- consultas ---
	CheckOnWhatsApp(ctx context.Context, phones []string) ([]OnWhatsApp, error)
	GetUserInfo(ctx context.Context, jids []string) ([]UserInfo, error)
	GetProfilePicture(ctx context.Context, jid string, preview bool) (string, error)

	// --- grupos ---
	ListGroups(ctx context.Context) ([]Group, error)
	GroupInfo(ctx context.Context, jid string) (Group, error)
	CreateGroup(ctx context.Context, name string, participants []string) (Group, error)
	LeaveGroup(ctx context.Context, jid string) error
	UpdateParticipants(ctx context.Context, jid string, action ParticipantAction, participants []string) ([]GroupParticipant, error)
	SetGroupName(ctx context.Context, jid, name string) error
	SetGroupTopic(ctx context.Context, jid, topic string) error
	GroupInviteLink(ctx context.Context, jid string, reset bool) (string, error)
	JoinGroupWithLink(ctx context.Context, code string) (string, error)
}

// MediaSink recebe binarios de midia ja descriptografados para armazenar.
// Implementado quando MEDIA_BACKEND != none; pode ser nil.
type MediaSink interface {
	Enabled() bool
	// Store guarda o binario e devolve a URL relativa de download.
	Store(ctx context.Context, session, msgID, mimetype string, data []byte) (url string, size int, err error)
}

// Deps e o que o gateway injeta em cada engine.
type Deps struct {
	Session string
	// DSN do Postgres; a engine usa para o store proprio dela (ex.: sqlstore).
	DSN    string
	Logger *slog.Logger
	// Emit publica um evento canonico no barramento interno.
	Emit func(events.Event)
	// Media, se != nil e Enabled(), faz a engine baixar+guardar a midia
	// recebida e anexar um campo "media" no payload da mensagem.
	Media MediaSink
	// RawEvents diz se o payload de mensagem/recibo deve incluir "raw" (o
	// struct cru da engine). Default (nil): nao inclui.
	RawEvents func() bool
}

type Factory func(deps Deps) (Engine, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	registry[name] = f
}

func Get(name string) (Factory, bool) {
	mu.RLock()
	defer mu.RUnlock()
	f, ok := registry[name]
	return f, ok
}
