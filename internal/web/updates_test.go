package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateCheck(t *testing.T) {
	tag := "v1.1.0"
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://github.com/kauredo/hallstream/releases/tag/` + tag + `"}`))
	}))
	defer gh.Close()
	s := newServer(t)
	s.Version, s.UpdateURL = "1.0.0", gh.URL

	var st struct {
		Update *struct{ Version, URL string } `json:"update"`
	}
	s.checkUpdate() // the check is off: what it finds stays hidden
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Update != nil {
		t.Fatalf("check off: %+v", st.Update)
	}

	set := s.Store.Settings()
	set.UpdateCheck = true
	call(t, s, "PUT", "/api/settings", set, nil)
	select {
	case <-s.updateNow:
	default:
		t.Error("switching the check on did not wake RunUpdates")
	}
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Update == nil || st.Update.Version != "1.1.0" {
		t.Fatalf("newer release: %+v", st.Update)
	}

	tag = "v1.0.0"
	s.checkUpdate()
	st.Update = nil
	call(t, s, "GET", "/api/status", nil, &st)
	if st.Update != nil {
		t.Errorf("same version: %+v", st.Update)
	}
}
