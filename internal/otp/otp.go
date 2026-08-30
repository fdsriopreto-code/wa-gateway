// Package otp é um serviço enxuto de código de verificação por WhatsApp
// (estilo "Verify" de SaaS): gera um código numérico, guarda só o HMAC no
// Redis com TTL + teto de tentativas + cooldown de reenvio, e devolve
// verdadeiro/falso na conferência. Um código ativo por (sessão, número).
package otp

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"wa-gateway/internal/cache"
)

// Defaults — usados quando nem a request nem config.otp definem o campo.
const (
	defCodeLen     = 6
	defTTL         = 5 * time.Minute
	defMaxAttempts = 5
	defResendAfter = 60 * time.Second
	defHourlyCap   = 5
	defTemplate    = "{{code}} é o seu código de verificação{{brand}}. Expira em {{minutes}} min. Não compartilhe."
)

// Options resolve os parâmetros de um envio (0/"" = default).
type Options struct {
	Template    string
	Brand       string
	CodeLength  int
	TTL         time.Duration
	MaxAttempts int
	ResendAfter time.Duration
	HourlyCap   int
}

func (o Options) codeLen() int {
	if o.CodeLength >= 4 && o.CodeLength <= 10 {
		return o.CodeLength
	}
	return defCodeLen
}
func (o Options) ttl() time.Duration {
	if o.TTL >= 30*time.Second && o.TTL <= 30*time.Minute {
		return o.TTL
	}
	return defTTL
}
func (o Options) maxAttempts() int {
	if o.MaxAttempts >= 1 && o.MaxAttempts <= 10 {
		return o.MaxAttempts
	}
	return defMaxAttempts
}
func (o Options) resendAfter() time.Duration {
	if o.ResendAfter > 0 {
		return o.ResendAfter
	}
	return defResendAfter
}
func (o Options) hourlyCap() int {
	if o.HourlyCap < 0 {
		return 0
	}
	if o.HourlyCap == 0 {
		return defHourlyCap
	}
	return o.HourlyCap
}
func (o Options) template() string {
	if strings.TrimSpace(o.Template) != "" {
		return o.Template
	}
	return defTemplate
}

// Challenge é o que o /otp/send devolve.
type Challenge struct {
	ID          string    `json:"id"`
	To          string    `json:"to"`
	Message     string    `json:"-"` // texto renderizado (o handler manda pela engine)
	ExpiresAt   time.Time `json:"expiresAt"`
	ResendAfter int       `json:"resendAfterSeconds"`
	code        string
}

// Result é o veredito de uma conferência.
type Result struct {
	Valid        bool   `json:"valid"`
	Reason       string `json:"reason,omitempty"` // mismatch|expired|locked|not_found
	AttemptsLeft int    `json:"attemptsLeft"`
}

// Errors "de negócio" que o handler mapeia pra HTTP.
type RateError struct {
	RetryAfter int
	Reason     string // cooldown|hourly_cap
}

func (e *RateError) Error() string {
	return fmt.Sprintf("otp: %s (retry em %ds)", e.Reason, e.RetryAfter)
}

type Service struct {
	rc  *cache.Redis
	mac []byte
}

// New — secretKeyHex é a SECRET_KEY (64 hex). Vazio => pepper fixo (funciona,
// mas defina SECRET_KEY em produção).
func New(rc *cache.Redis, secretKeyHex string) *Service {
	mac := []byte(strings.TrimSpace(secretKeyHex))
	if len(mac) == 0 {
		mac = []byte("wa-gateway/otp/v1/fallback-pepper")
	}
	return &Service{rc: rc, mac: mac}
}

func (s *Service) hash(session, to, code string) string {
	m := hmac.New(sha256.New, s.mac)
	m.Write([]byte(session + "|" + to + "|" + code))
	return hex.EncodeToString(m.Sum(nil))
}

func key(session, to string) string    { return "wa:otp:" + session + ":" + to }
func idKey(id string) string           { return "wa:otp:id:" + id }
func capKey(session, to string) string { return "wa:otp:cap:" + session + ":" + to }
func cdKey(session, to string) string  { return "wa:otp:cd:" + session + ":" + to }

// Digits normaliza um telefone/JID para só os dígitos (chave do OTP).
func Digits(s string) string {
	if i := strings.IndexByte(s, '@'); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func randomCode(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	for i := range b {
		x, _ := rand.Int(rand.Reader, big.NewInt(10))
		b[i] = digits[x.Int64()]
	}
	return string(b)
}

// Send gera e persiste um código para (session, to). Não envia nada — devolve
// Challenge com a mensagem renderizada pro handler mandar pela engine.
func (s *Service) Send(ctx context.Context, session, to string, o Options) (*Challenge, error) {
	digits := Digits(to)
	if digits == "" {
		return nil, fmt.Errorf("otp: número inválido")
	}
	k := key(session, digits)
	rdb := s.rc.Raw()
	now := time.Now()

	// cooldown de reenvio — chave dedicada com TTL no Redis
	if ra := o.resendAfter(); ra > 0 {
		ok, err := rdb.SetNX(ctx, cdKey(session, digits), "1", ra).Result()
		if err != nil {
			return nil, err
		}
		if !ok {
			ttl, _ := rdb.TTL(ctx, cdKey(session, digits)).Result()
			sec := int(ttl.Seconds())
			if sec < 1 {
				sec = int(ra.Seconds())
			}
			return nil, &RateError{RetryAfter: sec, Reason: "cooldown"}
		}
	}

	// contador de reenvios (informativo)
	prev, _ := rdb.HGetAll(ctx, k).Result()

	// teto por hora
	if cap := o.hourlyCap(); cap > 0 {
		n, err := rdb.Incr(ctx, capKey(session, digits)).Result()
		if err != nil {
			return nil, err
		}
		if n == 1 {
			_ = rdb.Expire(ctx, capKey(session, digits), time.Hour).Err()
		}
		if int(n) > cap {
			ttl, _ := rdb.TTL(ctx, capKey(session, digits)).Result()
			ra := int(ttl.Seconds())
			if ra < 1 {
				ra = 3600
			}
			return nil, &RateError{RetryAfter: ra, Reason: "hourly_cap"}
		}
	}

	code := randomCode(o.codeLen())
	ttl := o.ttl()
	exp := now.Add(ttl)
	id := "otp_" + randHex(9)
	sends := 1
	if s0, e := strconv.Atoi(prev["sends"]); e == nil {
		sends = s0 + 1
	}

	pipe := rdb.TxPipeline()
	pipe.Del(ctx, k)
	pipe.HSet(ctx, k, map[string]any{
		"id": id, "hash": s.hash(session, digits, code), "exp": exp.Unix(),
		"attempts": 0, "max": o.maxAttempts(), "sends": sends,
	})
	pipe.Expire(ctx, k, ttl)
	pipe.Set(ctx, idKey(id), digits+"|"+session, ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	return &Challenge{
		ID: id, To: digits, Message: render(o.template(), code, o.Brand, ttl),
		ExpiresAt: exp, ResendAfter: int(o.resendAfter().Seconds()), code: code,
	}, nil
}

var verifyLua = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return {'not_found','0'} end
local now = tonumber(ARGV[2])
if now >= tonumber(redis.call('HGET', KEYS[1], 'exp')) then
  redis.call('DEL', KEYS[1]); return {'expired','0'}
end
local att = tonumber(redis.call('HGET', KEYS[1], 'attempts'))
local mx  = tonumber(redis.call('HGET', KEYS[1], 'max'))
if att >= mx then redis.call('DEL', KEYS[1]); return {'locked','0'} end
if redis.call('HGET', KEYS[1], 'hash') == ARGV[1] then
  redis.call('DEL', KEYS[1]); return {'ok','0'}
end
local n = redis.call('HINCRBY', KEYS[1], 'attempts', 1)
return {'mismatch', tostring(mx - n)}
`)

// Verify confere o código. `to` pode ser número/JID; se vazio, usa `id`.
func (s *Service) Verify(ctx context.Context, session, to, id, code string) (Result, error) {
	digits := Digits(to)
	if digits == "" && id != "" {
		v, err := s.rc.Raw().Get(ctx, idKey(id)).Result()
		if err == redis.Nil {
			return Result{Reason: "not_found"}, nil
		}
		if err != nil {
			return Result{}, err
		}
		parts := strings.SplitN(v, "|", 2)
		digits = parts[0]
		if len(parts) == 2 && session == "" {
			session = parts[1]
		}
	}
	if digits == "" {
		return Result{Reason: "not_found"}, nil
	}
	code = strings.TrimSpace(code)
	cand := s.hash(session, digits, code)

	res, err := verifyLua.Run(ctx, s.rc.Raw(), []string{key(session, digits)}, cand, time.Now().Unix()).Result()
	if err != nil {
		return Result{}, err
	}
	arr, _ := res.([]any)
	reason, _ := arr[0].(string)
	left := 0
	if len(arr) > 1 {
		if ls, ok := arr[1].(string); ok {
			left, _ = strconv.Atoi(ls)
		}
	}
	if reason == "ok" {
		_ = s.rc.Raw().Del(ctx, capKey(session, digits), cdKey(session, digits)).Err()
		return Result{Valid: true, AttemptsLeft: left}, nil
	}
	return Result{Valid: false, Reason: reason, AttemptsLeft: left}, nil
}

// Cancel apaga um código ativo e libera o cooldown (ex.: o SaaS abortou o
// fluxo, ou o envio pela engine falhou).
func (s *Service) Cancel(ctx context.Context, session, to string) error {
	d := Digits(to)
	return s.rc.Raw().Del(ctx, key(session, d), cdKey(session, d)).Err()
}

func render(tmpl, code, brand string, ttl time.Duration) string {
	brandTxt := ""
	if b := strings.TrimSpace(brand); b != "" {
		brandTxt = " " + b
	}
	r := strings.NewReplacer(
		"{{code}}", code,
		"{{brand}}", brandTxt,
		"{{minutes}}", strconv.Itoa(int(ttl.Minutes())),
		"{{ttl}}", ttl.String(),
	)
	return r.Replace(tmpl)
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
