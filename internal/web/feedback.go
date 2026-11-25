package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/kauredo/hallstream/internal/i18n"
)

// feedback is POST /api/feedback: the Ajuda form, sent on to the feedback
// worker (FeedbackURL), which files it as a GitHub issue. The program adds
// what a report always needs: its version, the system, the scoring input and
// the sport. Nothing is sent unless the person presses Enviar.
func (s *Server) feedback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type        string `json:"type"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if !decode(w, r, &req) {
		return
	}
	if s.FeedbackURL == "" {
		fail(w, http.StatusServiceUnavailable, i18n.T("api.feedback.sem_servico"))
		return
	}
	req.Title, req.Description = strings.TrimSpace(req.Title), strings.TrimSpace(req.Description)
	if req.Title == "" || req.Description == "" {
		fail(w, http.StatusBadRequest, i18n.T("api.feedback.falta"))
		return
	}
	if req.Type != "bug" && req.Type != "feature" {
		req.Type = "feedback"
	}
	set := s.Store.Settings()
	sport := set.Club.Sport
	if sport == "" {
		sport = "basketball"
	}
	body, _ := json.Marshal(map[string]string{
		"type": req.Type, "title": req.Title, "description": req.Description,
		"appVersion": s.Version, "os": runtime.GOOS + "/" + runtime.GOARCH,
		"scoring": set.Input, "sport": sport,
	})
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out, err := http.NewRequestWithContext(ctx, "POST", s.FeedbackURL, bytes.NewReader(body))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(out)
	if err != nil {
		fail(w, http.StatusBadGateway, i18n.T("api.feedback.sem_ligacao"))
		return
	}
	defer res.Body.Close()
	var got struct {
		IssueURL string `json:"issueUrl"`
		Error    string `json:"error"`
	}
	json.NewDecoder(res.Body).Decode(&got)
	if res.StatusCode != http.StatusCreated || !strings.HasPrefix(got.IssueURL, "https://github.com/") {
		s.Log(i18n.T("log.feedback.recusado", "estado", res.Status, "erro", got.Error))
		fail(w, http.StatusBadGateway, i18n.T("api.feedback.recusado"))
		return
	}
	s.Log(i18n.T("log.feedback.enviado", "url", got.IssueURL))
	reply(w, map[string]string{"issue_url": got.IssueURL})
}
