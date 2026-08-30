// Package events define o evento canonico interno e o barramento pub/sub
// in-process. Os nomes seguem o padrao "dominio.acao" (estilo WAHA).
package events

import "time"

// Event e a forma canonica de qualquer acontecimento de uma sessao,
// independente da engine que o originou.
type Event struct {
	ID        string    `json:"id"`
	Session   string    `json:"session"`
	Name      string    `json:"event"`
	Timestamp time.Time `json:"timestamp"`
	Engine    string    `json:"engine"`
	Payload   any       `json:"payload"`
}

// Nomes de alto nivel, estaveis, que o gateway garante emitir.
// A engine tambem repassa TODOS os eventos crus dela: os que nao
// tem mapeamento aqui saem como "engine.<TipoGo>".
const (
	SessionStatus    = "session.status"    // payload: {status, reason}
	QRCode           = "session.qr"        // payload: {code}
	PairSuccess      = "session.pair"      // payload: cru da engine
	SessionUnhealthy = "session.unhealthy" // payload: {status, since, forSeconds}
	SessionHealthy   = "session.healthy"   // payload: {status} — recuperou

	Message         = "message"     // RECEBIDA (nao fromMe), payload normalizado
	MessageAny      = "message.any" // qualquer, inclusive fromMe, payload normalizado
	MessageAck      = "message.ack" // recibo entrega/leitura, payload normalizado
	MessageReaction = "message.reaction"
	MessageRevoked  = "message.revoked"
	MessageEdited   = "message.edited"

	Presence     = "presence.update"
	ChatPresence = "chat.presence" // typing/recording
	GroupUpdate  = "group.update"
	GroupJoin    = "group.join"
	Picture      = "contact.picture"
	Contact      = "contact.update"
	CallReceived = "call.received"
	CallAccepted = "call.accepted"
	CallRejected = "call.rejected"
	HistorySync  = "history.sync"
)
