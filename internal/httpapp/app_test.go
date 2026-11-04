package httpapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSessionRejectsInvalidRequestsBeforeWriting(t *testing.T) {
	handler := New(nil, t.TempDir(), true)
	for _, tc := range []struct {
		name, body, origin, contentType string
		want                            int
	}{
		{"another origin", `{"displayName":"Robin"}`, "https://another.example", "application/json", http.StatusForbidden},
		{"form request", "displayName=Robin", "", "application/x-www-form-urlencoded", http.StatusUnsupportedMediaType},
		{"empty name", `{"displayName":"  "}`, "", "application/json", http.StatusBadRequest},
		{"oversized name", `{"displayName":"` + strings.Repeat("雪", 25) + `"}`, "", "application/json", http.StatusBadRequest},
		{"trailing object", `{"displayName":"Robin"} {}`, "", "application/json", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "https://game.example/api/session", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, r)
			if recorder.Code != tc.want {
				t.Errorf("status %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}
