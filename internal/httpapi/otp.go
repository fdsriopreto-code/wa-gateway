package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"wa-gateway/internal/engine"
	"wa-gateway/internal/observability"
	"wa-gateway/internal/otp"
)

type otpSendReq struct {
	To           string          `json:"to"`
	Template     string          `json:"template"`
	Brand        string          `json:"brand"`
	CodeLength   int             `json:"codeLength"`
	TTLSeconds   int             `json:"ttlSeconds"`
	CallbackURL  string          `json:"callbackUrl"`  // POST quando a msg for entregue/lida/falhar
	CallbackData json.RawMessage `json:"callbackData"` // ecoado no callback
}

// otpSend gera um código, guarda o hash no Redis e manda pela sessão.
func (d Deps) otpSend(w http.ResponseWriter, r *http.Request) {
	if d.OTP == nil {
		writeErr(w, http.StatusNotImplemented, "not_supported", "OTP desligado (sem Redis)")
		return
	}
	session := chi.URLParam(r, "session")
	var req otpSendReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if otp.Digits(req.To) == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "to (número) é obrigatório")
		return
	}

	opts := d.otpOptions(session)
	if req.Template != "" {
		opts.Template = req.Template
	}
	if req.Brand != "" {
		opts.Brand = req.Brand
	}
	if req.CodeLength > 0 {
		opts.CodeLength = req.CodeLength
	}
	if req.TTLSeconds > 0 {
		opts.TTL = time.Duration(req.TTLSeconds) * time.Second
	}

	// a sessão precisa estar viva ANTES de queimar um código.
	eng, ok := d.engineFor(w, session)
	if !ok {
		return
	}

	ch, err := d.OTP.Send(r.Context(), session, req.To, opts)
	if err != nil {
		var re *otp.RateError
		if errors.As(err, &re) {
			w.Header().Set("Retry-After", strconv.Itoa(re.RetryAfter))
			writeErr(w, http.StatusTooManyRequests, "rate_limited",
				"aguarde para reenviar ("+re.Reason+"): "+strconv.Itoa(re.RetryAfter)+"s")
			return
		}
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	res, serr := eng.SendText(r.Context(), ch.To+"@s.whatsapp.net", ch.Message, engine.MessageOpts{})
	if serr != nil {
		_ = d.OTP.Cancel(r.Context(), session, ch.To) // não deixa código órfão
		writeErr(w, http.StatusBadGateway, "send_failed", "não consegui enviar a mensagem: "+serr.Error())
		return
	}
	observability.OTPSent.WithLabelValues(session).Inc()
	if req.CallbackURL != "" {
		d.armCB(r.Context(), session, res, cbOpts{CallbackURL: req.CallbackURL, CallbackData: req.CallbackData})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 ch.ID,
		"to":                 ch.To,
		"expiresAt":          ch.ExpiresAt.UTC().Format(time.RFC3339),
		"resendAfterSeconds": ch.ResendAfter,
	})
}

type otpVerifyReq struct {
	To   string `json:"to"`
	ID   string `json:"id"`
	Code string `json:"code"`
}

func (d Deps) otpVerify(w http.ResponseWriter, r *http.Request) {
	if d.OTP == nil {
		writeErr(w, http.StatusNotImplemented, "not_supported", "OTP desligado (sem Redis)")
		return
	}
	session := chi.URLParam(r, "session")
	var req otpVerifyReq
	if err := decode(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Code == "" || (req.To == "" && req.ID == "") {
		writeErr(w, http.StatusBadRequest, "bad_request", "code e (to ou id) são obrigatórios")
		return
	}
	res, err := d.OTP.Verify(r.Context(), session, req.To, req.ID, req.Code)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	result := res.Reason
	if res.Valid {
		result = "ok"
	}
	observability.OTPVerified.WithLabelValues(session, result).Inc()

	status := http.StatusOK
	if !res.Valid {
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, res)
}

func (d Deps) otpCancel(w http.ResponseWriter, r *http.Request) {
	if d.OTP == nil {
		writeErr(w, http.StatusNotImplemented, "not_supported", "OTP desligado (sem Redis)")
		return
	}
	session := chi.URLParam(r, "session")
	var req struct {
		To string `json:"to"`
	}
	if err := decode(r, &req); err != nil || req.To == "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "to é obrigatório")
		return
	}
	if err := d.OTP.Cancel(r.Context(), session, req.To); err != nil {
		writeErr(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cancelled": true})
}

// otpOptions monta os defaults a partir de config.otp da sessão.
func (d Deps) otpOptions(session string) otp.Options {
	var o otp.Options
	c := d.Manager.OTPConfig(session)
	if c == nil {
		return o
	}
	o.Template = c.Template
	o.Brand = c.Brand
	o.CodeLength = c.CodeLength
	if c.TTLSeconds > 0 {
		o.TTL = time.Duration(c.TTLSeconds) * time.Second
	}
	o.MaxAttempts = c.MaxAttempts
	if c.ResendAfterSeconds > 0 {
		o.ResendAfter = time.Duration(c.ResendAfterSeconds) * time.Second
	}
	o.HourlyCap = c.HourlyCap
	return o
}
