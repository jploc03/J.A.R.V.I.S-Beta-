package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitHubFallback(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Error("missing version header")
		}
		if r.URL.Path == "/new" {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":{"code":"InternalError","message":"boom"}}`))
			return
		}
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"hello"}}]}`))
	}))
	defer srv.Close()
	ghEndpoint, ghLegacyEndpoint = srv.URL+"/new", srv.URL+"/old"
	ans, err := githubChat("tok", "", []ollamaMsg{{Role: "user", Content: "hi"}})
	if err != nil || ans != "hello" || calls != 2 {
		t.Fatalf("ans=%q err=%v calls=%d", ans, err, calls)
	}
	ghLegacyEndpoint = srv.URL + "/new"
	_, err = githubChat("tok", "", []ollamaMsg{{Role: "user", Content: "hi"}})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err=%v", err)
	}
}
