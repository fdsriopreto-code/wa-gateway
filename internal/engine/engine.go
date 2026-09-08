// Package engine define o contrato que qualquer camada de WhatsApp deve
// cumprir (hoje: whatsmeow) e um registry de factories.
package engine

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"

	"wa-gateway/internal/events"
)

// ErrNotSupported: a operação não existe neste motor (ex.: grupos na Cloud
// API, botões interativos no whatsmeow). Os handlers HTTP mapeiam para 501.
var ErrNotSupported = errors.New("operação não suportada por este motor")

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

// MessageOpts sao extras comuns a qualquer envio: citar mensagem, mencionar
// participantes e (para texto) tentar anexar preview de link.
type MessageOpts struct {
	QuotedID          string   `json:"quotedId,omitempty"`          // stanza id da msg citada
	QuotedParticipant string   `json:"quotedParticipant,omitempty"` // jid do autor citado (grupo)
	QuotedText        string   `json:"quotedText,omitempty"`        // texto da msg citada (preview)
	Mentions          []string `json:"mentions,omitempty"`          // numeros/jids mencionados
	LinkPreview       bool     `json:"linkPreview,omitempty"`       // texto: busca OG do 1o link
	Forwarded         bool     `json:"forwarded,omitempty"`         // marca como encaminhada
}

// ForwardSource descreve a mensagem guardada que sera encaminhada.
type ForwardSource struct {
	Type  string
	Body  string
	Media *StoredMedia
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
	Opts     MessageOpts
}

// StoredMedia carrega os campos necessarios para baixar uma midia depois,
// a partir do que ficou guardado (sem precisar do evento original).
type StoredMedia struct {
	Type          string // image|video|audio|document|sticker
	DirectPath    string
	Mimetype      string
	Filename      string
	MediaKey      []byte
	FileEncSHA256 []byte
	FileSHA256    []byte
	FileLength    uint64
	CloudID       string // id de mídia da Cloud API (quando engine=cloud)
}

// Me e o perfil da propria sessao.
type Me struct {
	JID          string   `json:"jid"`
	LID          string   `json:"lid,omitempty"`
	PushName     string   `json:"pushName,omitempty"`
	Platform     string   `json:"platform,omitempty"`
	BusinessName string   `json:"businessName,omitempty"`
	Devices      []string `json:"devices,omitempty"`
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

// --- mensagens interativas / templates (WhatsApp Cloud API) ---

// Interactive e uma mensagem com botões, lista ou CTA de URL. Só o motor
// Cloud API renderiza isso de forma confiável.
type Interactive struct {
	Type       string        `json:"type"` // "button" | "list" | "cta_url"
	Body       string        `json:"body"`
	Header     string        `json:"header,omitempty"`
	Footer     string        `json:"footer,omitempty"`
	Buttons    []Button      `json:"buttons,omitempty"`    // type=button (máx 3)
	ButtonText string        `json:"buttonText,omitempty"` // type=list: rótulo do menu
	Sections   []ListSection `json:"sections,omitempty"`   // type=list
	URL        string        `json:"url,omitempty"`        // type=cta_url
	DisplayURL string        `json:"displayUrl,omitempty"` // type=cta_url: rótulo do botão
}

type Button struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type ListSection struct {
	Title string    `json:"title,omitempty"`
	Rows  []ListRow `json:"rows"`
}

type ListRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// Label e uma etiqueta do WhatsApp Business.
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color int32  `json:"color"`
}

// ContactEntry e uma entrada da agenda da sessao (contact store do whatsmeow).
type ContactEntry struct {
	JID          string `json:"jid"`
	Phone        string `json:"phone,omitempty"`
	FirstName    string `json:"firstName,omitempty"`
	FullName     string `json:"fullName,omitempty"`
	PushName     string `json:"pushName,omitempty"`
	BusinessName string `json:"businessName,omitempty"`
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
	SendText(ctx context.Context, chatID, text string, opts MessageOpts) (SendResult, error)
	SendImage(ctx context.Context, chatID string, data []byte, mimetype, caption string) (SendResult, error)
	SendFile(ctx context.Context, chatID string, m Media) (SendResult, error)
	SendVideo(ctx context.Context, chatID string, m Media) (SendResult, error)
	SendAudio(ctx context.Context, chatID string, m Media) (SendResult, error)
	SendSticker(ctx context.Context, chatID string, data []byte, opts MessageOpts) (SendResult, error)
	SendLocation(ctx context.Context, chatID string, loc Location) (SendResult, error)
	SendContact(ctx context.Context, chatID string, cs []Contact) (SendResult, error)
	SendPoll(ctx context.Context, chatID, name string, options []string, selectable int, opts MessageOpts) (SendResult, error)
	Forward(ctx context.Context, toChatID string, src ForwardSource) (SendResult, error)
	// SendInteractive: botões/lista/CTA. ErrNotSupported no whatsmeow.
	SendInteractive(ctx context.Context, chatID string, i Interactive) (SendResult, error)
	// SendTemplate: mensagem de template aprovada (Cloud API). components é o
	// array "components" cru da Graph API. ErrNotSupported no whatsmeow.
	SendTemplate(ctx context.Context, chatID, name, lang string, components json.RawMessage) (SendResult, error)

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
	Contacts(ctx context.Context) ([]ContactEntry, error)

	// --- labels (WhatsApp Business) ---
	Labels(ctx context.Context) ([]Label, error)
	EditLabel(ctx context.Context, labelID, name string, color int32, deleted bool) error
	SetChatLabel(ctx context.Context, chatID, labelID string, on bool) error
	SetMessageLabel(ctx context.Context, chatID, messageID, labelID string, on bool) error
	DownloadMedia(ctx context.Context, m StoredMedia) (data []byte, mimetype string, err error)

	// --- perfil / conta ---
	PairPhone(ctx context.Context, phone string) (string, error)
	Me(ctx context.Context) (Me, error)
	SetStatusMessage(ctx context.Context, text string) error
	SetPresence(ctx context.Context, available bool) error
	SetBlocked(ctx context.Context, jid string, block bool) ([]string, error)
	Blocklist(ctx context.Context) ([]string, error)

	// --- grupos ---
	ListGroups(ctx context.Context) ([]Group, error)
	GroupInfo(ctx context.Context, jid string) (Group, error)
	CreateGroup(ctx context.Context, name string, participants []string) (Group, error)
	LeaveGroup(ctx context.Context, jid string) error
	UpdateParticipants(ctx context.Context, jid string, action ParticipantAction, participants []string) ([]GroupParticipant, error)
	SetGroupName(ctx context.Context, jid, name string) error
	SetGroupTopic(ctx context.Context, jid, topic string) error
	SetGroupPhoto(ctx context.Context, jid string, data []byte) (string, error)
	SetGroupAnnounce(ctx context.Context, jid string, on bool) error
	SetGroupLocked(ctx context.Context, jid string, on bool) error
	GroupInviteLink(ctx context.Context, jid string, reset bool) (string, error)
	JoinGroupWithLink(ctx context.Context, code string) (string, error)
}

// MediaSink recebe binarios de midia ja descriptografados para armazenar.
// Implementado quando MEDIA_BACKEND != none; pode ser nil.
type MediaSink interface {
	Enabled() bool
	// WantStore diz se a midia recebida desta sessao deve ser baixada e
	// guardada (a sessao pode ter optado por descartar via config.media.store).
	WantStore(session string) bool
	// Store guarda o binario e devolve a URL relativa de download. url vazio
	// (sem erro) = a sessao optou por nao guardar.
	Store(ctx context.Context, session, msgID, mimetype string, data []byte) (url string, size int, err error)
}

// PollStore persiste opcoes e o placar corrente dos votos de uma enquete,
// pra resolver o texto votado no evento message.poll_vote.
type PollStore interface {
	// SavePollOptions grava as opcoes EXATAS (ordem preservada, sem normalizar).
	SavePollOptions(ctx context.Context, session, pollID string, options []string) error
	// PollOptions devolve as opcoes guardadas (nil, nil se nao achou).
	PollOptions(ctx context.Context, session, pollID string) ([]string, error)
	// RecordVote atualiza o placar: voter -> opcoes selecionadas agora
	// (selected vazio = removeu o voto).
	RecordVote(ctx context.Context, session, pollID, voter string, selected []string) error
}

// AutoBehavior sao comportamentos automaticos que a sessao pode ligar.
type AutoBehavior struct {
	// AutoRead marca como lida toda mensagem recebida (envia recibo azul).
	AutoRead bool
	// AutoOnline mantem a sessao com presenca "available" apos conectar.
	AutoOnline bool
}

// CloudConfig e a configuracao de uma sessao que fala a WhatsApp Cloud API
// oficial da Meta em vez do protocolo Web (whatsmeow).
type CloudConfig struct {
	PhoneNumberID string `json:"phoneNumberId"`
	AccessToken   string `json:"accessToken"`
	WABAID        string `json:"wabaId,omitempty"`
	GraphVersion  string `json:"graphVersion,omitempty"` // default "v21.0"
	VerifyToken   string `json:"verifyToken,omitempty"`  // GET do webhook (hub.verify_token)
	AppSecret     string `json:"appSecret,omitempty"`    // valida X-Hub-Signature-256
}

// Deps e o que o gateway injeta em cada engine.
type Deps struct {
	Session string
	// Cloud, se != nil, e a config do motor Cloud API desta sessao.
	Cloud *CloudConfig
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
	// Behavior devolve flags de comportamento automatico da sessao
	// (auto-read, presenca online). Default (nil): tudo desligado.
	Behavior func() AutoBehavior
	// Enrich, se != nil, transcreve audio / descreve imagem recebidos e
	// devolve campos extras pro payload (transcript / imageCaption). Recebe
	// os bytes ja baixados. nil = desligado.
	Enrich func(ctx context.Context, mediaType, mime string, data []byte) map[string]any
	// PollStore guarda as opcoes de uma enquete (por messageId) e o placar
	// corrente dos votos, pra decifrar/agregar votos de enquete depois.
	// nil = feature desligada (poll_vote sai sem opcao resolvida).
	PollStore PollStore
	// StoredJID e o JID que esta sessao ja pareou (coluna sessions.jid),
	// vazio para sessao nova. A engine usa para carregar o device certo
	// quando varias sessoes compartilham o mesmo store.
	StoredJID string
	// Recovering indica que a sessao esta sendo retomada no boot (RestoreOwned)
	// e nao iniciada interativamente. So nesse caso a engine pode adotar um
	// device orfao do store quando StoredJID esta vazio.
	Recovering bool
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
