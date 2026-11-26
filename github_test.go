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

func TestGitHubRedirectKeepsPost(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/new":
			http.Redirect(w, r, "/moved", http.StatusFound)
		case "/moved":
			if r.Method != "POST" {
				w.Write([]byte("OK"))
				return
			}
			w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Jarvis \"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"online.\"}}]}\n\ndata: [DONE]\n"))
		default:
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	old := http.DefaultTransport
	http.DefaultTransport = srv.Client().Transport
	defer func() { http.DefaultTransport = old }()
	ghEndpoint, ghLegacyEndpoint = srv.URL+"/new", srv.URL+"/old"
	ans, err := githubChat("tok", "", []ollamaMsg{{Role: "user", Content: "hi"}})
	if err != nil || ans != "Jarvis online." {
		t.Fatalf("ans=%q err=%v", ans, err)
	}
}
