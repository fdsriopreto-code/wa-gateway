package whatsmeow

// Vigia da conexão: às vezes o socket com o WhatsApp trava em silêncio — o status
// segue "WORKING", nenhum Disconnected/KeepAliveTimeout chega e nenhuma mensagem
// entra (aconteceu em 09/10: 30 h sem eventos até um restart manual). Aqui anotamos
// a hora do último evento e, se a sessão ficar tempo demais sem nada (nem status de
// contatos, recibos, presença), reconectamos o mesmo client — sem logout, sem QR.

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.mau.fi/whatsmeow"

	"wa-gateway/internal/engine"
)

// limiteSilencio: WA_WATCHDOG_MIN (minutos, padrão 45; 0 desliga).
func limiteSilencio() time.Duration {
	min := 45
	if v, err := strconv.Atoi(os.Getenv("WA_WATCHDOG_MIN")); err == nil && v >= 0 {
		min = v
	}
	return time.Duration(min) * time.Minute
}

func (e *Engine) marcarEvento() { e.ultimoEvento.Store(time.Now().UnixNano()) }

func (e *Engine) silencio() time.Duration {
	return time.Since(time.Unix(0, e.ultimoEvento.Load()))
}

// vigiar roda enquanto a sessão estiver ativa (ctx do Start).
func (e *Engine) vigiar(ctx context.Context, client *whatsmeow.Client) {
	limite := limiteSilencio()
	if limite <= 0 {
		return
	}
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		e.mu.RLock()
		atual, status := e.client, e.status
		e.mu.RUnlock()
		if atual != client || status != engine.StatusWorking {
			continue // parada, trocada ou já reconectando
		}
		if s := e.silencio(); s > limite {
			e.deps.Logger.Warn("vigia: sessão sem eventos do WhatsApp — reconectando sem logout",
				"session", e.deps.Session, "sem_eventos", s.Round(time.Minute).String())
			client.Disconnect()
			time.Sleep(2 * time.Second)
			e.marcarEvento() // dá o prazo inteiro de novo antes de tentar outra vez
			if err := client.Connect(); err != nil {
				e.deps.Logger.Warn("vigia: reconexão falhou", "session", e.deps.Session, "err", err.Error())
			}
		}
	}
}
