package biostudies

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestSearch(t *testing.T) {
	resp := wireSearchResp{
		TotalHits: 100,
		Hits: []wireHit{
			{
				Accession:   "S-BSST3126",
				Type:        "study",
				Title:       "Cancer genomics study",
				Author:      "Smith J",
				Files:       3,
				Links:       5,
				ReleaseDate: "2024-01-15",
				Views:       42,
				IsPublic:    true,
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/biostudies/api/v1/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("q") == "" {
			t.Error("missing q param")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	// Override baseURL via the client's HTTP transport by pointing at test server.
	// We swap the baseURL indirectly: use a custom transport that rewrites host.
	origBase := baseURL
	_ = origBase // not exported; we test via Search directly by patching the URL

	// Use a small wrapper: patch the server URL into a request directly.
	// Since baseURL is a package-level const we can't change it, but we can
	// exercise the JSON parsing path by hitting srv directly.
	studies, total, err := c.searchURL(context.Background(), srv.URL+"/biostudies/api/v1/search?q=cancer&pageSize=10&page=1")
	if err != nil {
		t.Fatal(err)
	}
	if total != 100 {
		t.Errorf("total = %d, want 100", total)
	}
	if len(studies) != 1 {
		t.Fatalf("len(studies) = %d, want 1", len(studies))
	}
	s := studies[0]
	if s.ID != "S-BSST3126" {
		t.Errorf("ID = %q, want S-BSST3126", s.ID)
	}
	if s.Title != "Cancer genomics study" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.Files != 3 {
		t.Errorf("Files = %d, want 3", s.Files)
	}
}

func TestGetStudy(t *testing.T) {
	resp := wireStudy{
		Accno: "S-BSST3126",
		Attributes: []wireAttr{
			{Name: "Title", Value: "Cancer genomics study"},
			{Name: "Description", Value: "A study about cancer."},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/biostudies/api/v1/studies/S-BSST3126" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0

	s, err := c.getStudyURL(context.Background(), srv.URL+"/biostudies/api/v1/studies/S-BSST3126")
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "S-BSST3126" {
		t.Errorf("ID = %q, want S-BSST3126", s.ID)
	}
	if s.Title != "Cancer genomics study" {
		t.Errorf("Title = %q, want Cancer genomics study", s.Title)
	}
}
