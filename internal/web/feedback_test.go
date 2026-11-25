package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFeedback(t *testing.T) {
	var sent map[string]string
	status := http.StatusCreated
	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&sent)
		w.WriteHeader(status)
		w.Write([]byte(`{"success":true,"issueUrl":"https://github.com/kauredo/hallstream/issues/7"}`))
	}))
	defer worker.Close()

	s := newServer(t)
	form := map[string]string{"type": "bug", "title": " The clock froze ", "description": "In the second quarter."}
	if code := call(t, s, "POST", "/api/feedback", form, nil); code != 503 {
		t.Errorf("no worker: %d, want 503", code)
	}

	s.Version, s.FeedbackURL = "0.1.0", worker.URL
	var got struct {
		IssueURL string `json:"issue_url"`
	}
	if code := call(t, s, "POST", "/api/feedback", form, &got); code != 200 || got.IssueURL != "https://github.com/kauredo/hallstream/issues/7" {
		t.Fatalf("sent: %d %+v", code, got)
	}
	if sent["title"] != "The clock froze" || sent["appVersion"] != "0.1.0" || sent["scoring"] != "console" || sent["sport"] != "basketball" || sent["type"] != "bug" {
		t.Errorf("the worker got %v", sent)
	}

	if code := call(t, s, "POST", "/api/feedback", map[string]string{"title": "x", "description": " "}, nil); code != 400 {
		t.Errorf("no description: %d, want 400", code)
	}
	status = http.StatusBadGateway
	if code := call(t, s, "POST", "/api/feedback", form, nil); code != 502 {
		t.Errorf("worker refused: %d, want 502", code)
	}
}
