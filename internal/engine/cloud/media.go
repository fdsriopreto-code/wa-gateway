package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"wa-gateway/internal/engine"
)

// uploadMedia sobe bytes para /{phoneNumberID}/media e devolve o media id.
func (e *Engine) uploadMedia(ctx context.Context, data []byte, mimetype string) (string, error) {
	if mimetype == "" {
		mimetype = http.DetectContentType(data)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("messaging_product", "whatsapp")
	_ = mw.WriteField("type", mimetype)
	fw, err := mw.CreateFormFile("file", "upload")
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	_ = mw.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		e.base()+"/"+e.cfg.PhoneNumberID+"/media", &buf)
	req.Header.Set("Authorization", "Bearer "+e.cfg.AccessToken)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	var out struct {
		ID string `json:"id"`
	}
	if err := e.do(req, &out); err != nil {
		return "", err
	}
	if out.ID == "" {
		return "", fmt.Errorf("upload de mídia sem id na resposta")
	}
	return out.ID, nil
}

// fetchMedia resolve um media id para bytes+mimetype.
func (e *Engine) fetchMedia(ctx context.Context, mediaID string) ([]byte, string, error) {
	var meta struct {
		URL      string `json:"url"`
		MimeType string `json:"mime_type"`
	}
	if err := e.graphGET(ctx, "/"+mediaID, nil, &meta); err != nil {
		return nil, "", err
	}
	if meta.URL == "" {
		return nil, "", fmt.Errorf("mídia %s sem url", mediaID)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, meta.URL, nil)
	req.Header.Set("Authorization", "Bearer "+e.cfg.AccessToken)
	resp, err := e.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("download de mídia: http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 100<<20))
	return data, meta.MimeType, err
}

// DownloadMedia (contrato engine): usa StoredMedia.CloudID.
func (e *Engine) DownloadMedia(ctx context.Context, m engine.StoredMedia) ([]byte, string, error) {
	id := m.CloudID
	if id == "" {
		id = m.DirectPath // tolerância: alguns registros guardam o id aí
	}
	if id == "" {
		return nil, "", fmt.Errorf("sem CloudID para baixar")
	}
	return e.fetchMedia(ctx, id)
}

// Me: perfil de negócio + número.
func (e *Engine) Me(ctx context.Context) (engine.Me, error) {
	var num struct {
		VerifiedName  string `json:"verified_name"`
		DisplayNumber string `json:"display_phone_number"`
		QualityRating string `json:"quality_rating"`
	}
	if err := e.graphGET(ctx, "/"+e.cfg.PhoneNumberID, map[string]string{
		"fields": "verified_name,display_phone_number,quality_rating",
	}, &num); err != nil {
		return engine.Me{}, err
	}
	return engine.Me{
		JID:          num.DisplayNumber,
		PushName:     num.VerifiedName,
		Platform:     "cloud-api (quality " + num.QualityRating + ")",
		BusinessName: num.VerifiedName,
	}, nil
}

// SetStatusMessage: o "recado" do perfil de negócio (campo about).
func (e *Engine) SetStatusMessage(ctx context.Context, text string) error {
	return e.graphPOST(ctx, "/"+e.cfg.PhoneNumberID+"/whatsapp_business_profile", map[string]any{
		"messaging_product": "whatsapp",
		"about":             text,
	}, nil)
}
