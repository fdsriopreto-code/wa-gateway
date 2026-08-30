package whatsmeow

import (
	"context"
	"encoding/json"

	"wa-gateway/internal/engine"
)

// Mensagens interativas (botões/lista) e templates só funcionam de forma
// confiável pela WhatsApp Cloud API oficial. Pelo protocolo Web (whatsmeow)
// o WhatsApp não renderiza de forma estável — use uma sessão engine="cloud".

func (e *Engine) SendInteractive(context.Context, string, engine.Interactive) (engine.SendResult, error) {
	return engine.SendResult{}, engine.ErrNotSupported
}

func (e *Engine) SendTemplate(context.Context, string, string, string, json.RawMessage) (engine.SendResult, error) {
	return engine.SendResult{}, engine.ErrNotSupported
}
