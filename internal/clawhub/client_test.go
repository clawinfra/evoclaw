package clawhub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockServer returns an httptest.Server with registered handlers.
func mockServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	now := time.Now().UTC()

	// GET /v1/skills
	mux.HandleFunc("/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			resp := apiListResponse{
				Skills: []SkillMeta{
					{ID: "weather-v2", Name: "Weather", Version: "2.0.0", Author: "alex", Tags: []string{"weather"}, CreatedAt: now, UpdatedAt: now},
					{ID: "bird-cli", Name: "Bird CLI", Version: "1.1.0", Author: "alex", Tags: []string{"twitter"}, CreatedAt: now, UpdatedAt: now},
				},
				Total:  2,
				Limit:  20,
				Offset: 0,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// POST /v1/skills (publish)
		if r.Method == http.MethodPost {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-key" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var req PublishRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(PublishResponse{
				ID:      req.Skill.ID,
				Version: req.Skill.Version,
				URL:     "https://clawhub.com/skills/" + req.Skill.ID,
			})
			return
		}

		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	// GET /v1/skills/{id}
	mux.HandleFunc("/v1/skills/weather-v2", func(w http.ResponseWriter, r *http.Request) {
		skill := Skill{
			SkillMeta: SkillMeta{ID: "weather-v2", Name: "Weather", Version: "2.0.0", Author: "alex", CreatedAt: now, UpdatedAt: now},
			Files:     map[string]string{"SKILL.md": "# Weather Skill\nGet weather data."},
			Readme:    "# Weather Skill\nGet weather data.",
			Checksum:  "abc123",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(skill)
	})

	mux.HandleFunc("/v1/skills/not-found", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"skill not found"}`, http.StatusNotFound)
	})

	// GET /v1/skills/search
	mux.HandleFunc("/v1/skills/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		var skills []SkillMeta
		if q == "weather" {
			skills = []SkillMeta{
				{ID: "weather-v2", Name: "Weather", Version: "2.0.0", Author: "alex", CreatedAt: now, UpdatedAt: now},
			}
		}
		resp := apiListResponse{Skills: skills, Total: len(skills)}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	return httptest.NewServer(mux)
}

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient("", "")
	if c.baseURL != DefaultBaseURL {
		t.Errorf("expected default base URL, got %q", c.baseURL)
	}
}

func TestClawHubClient_ListSkills(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	skills, err := c.ListSkills(context.Background(), SkillFilter{})
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(skills))
	}
	if skills[0].ID != "weather-v2" {
		t.Errorf("unexpected first skill ID: %q", skills[0].ID)
	}
}

func TestClawHubClient_ListSkills_WithFilter(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	skills, err := c.ListSkills(context.Background(), SkillFilter{
		Author:   "alex",
		MinStars: 5,
		Limit:    10,
		SortBy:   "downloads",
		Tags:     []string{"weather"},
	})
	if err != nil {
		t.Fatalf("ListSkills with filter: %v", err)
	}
	// Mock returns all regardless of filter; just ensure no crash
	_ = skills
}

func TestClawHubClient_GetSkill(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	skill, err := c.GetSkill(context.Background(), "weather-v2")
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if skill.ID != "weather-v2" {
		t.Errorf("unexpected skill ID: %q", skill.ID)
	}
	if skill.Readme == "" {
		t.Error("expected non-empty Readme")
	}
}

func TestClawHubClient_GetSkill_EmptyID(t *testing.T) {
	c := NewClient(DefaultBaseURL, "")
	_, err := c.GetSkill(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestClawHubClient_GetSkill_NotFound(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	_, err := c.GetSkill(context.Background(), "not-found")
	if err == nil {
		t.Fatal("expected error for not-found skill")
	}
}

func TestClawHubClient_PublishSkill(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	resp, err := c.PublishSkill(context.Background(), Skill{
		SkillMeta: SkillMeta{
			ID:      "my-new-skill",
			Name:    "My New Skill",
			Version: "1.0.0",
			Author:  "alex",
		},
		Readme: "# My New Skill",
	})
	if err != nil {
		t.Fatalf("PublishSkill: %v", err)
	}
	if resp.ID != "my-new-skill" {
		t.Errorf("unexpected response ID: %q", resp.ID)
	}
	if resp.URL == "" {
		t.Error("expected non-empty URL in publish response")
	}
}

func TestClawHubClient_PublishSkill_MissingID(t *testing.T) {
	c := NewClient(DefaultBaseURL, "key")
	_, err := c.PublishSkill(context.Background(), Skill{})
	if err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestClawHubClient_PublishSkill_MissingName(t *testing.T) {
	c := NewClient(DefaultBaseURL, "key")
	_, err := c.PublishSkill(context.Background(), Skill{SkillMeta: SkillMeta{ID: "skill-id"}})
	if err == nil {
		t.Fatal("expected error for missing Name")
	}
}

func TestClawHubClient_PublishSkill_NoAPIKey(t *testing.T) {
	c := NewClient(DefaultBaseURL, "")
	_, err := c.PublishSkill(context.Background(), Skill{SkillMeta: SkillMeta{ID: "x", Name: "X"}})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestClawHubClient_PublishSkill_Unauthorized(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "wrong-key")
	_, err := c.PublishSkill(context.Background(), Skill{
		SkillMeta: SkillMeta{ID: "x", Name: "X", Version: "1.0.0"},
	})
	if err == nil {
		t.Fatal("expected error for unauthorized publish")
	}
}

func TestClawHubClient_SearchSkills(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	results, err := c.SearchSkills(context.Background(), "weather")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "weather-v2" {
		t.Errorf("unexpected result ID: %q", results[0].ID)
	}
}

func TestClawHubClient_SearchSkills_EmptyQuery(t *testing.T) {
	c := NewClient(DefaultBaseURL, "")
	_, err := c.SearchSkills(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestClawHubClient_SearchSkills_NoResults(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	results, err := c.SearchSkills(context.Background(), "nonexistent-skill-xyz")
	if err != nil {
		t.Fatalf("SearchSkills: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestClawHubClient_GetSkill_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key")
	_, err := c.GetSkill(context.Background(), "any")
	if err == nil {
		t.Fatal("expected error for bad JSON response")
	}
}

func TestClawHubClient_PublishSkill_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key")
	_, err := c.PublishSkill(context.Background(), Skill{SkillMeta: SkillMeta{ID: "x", Name: "X"}})
	if err == nil {
		t.Fatal("expected error for bad JSON in publish response")
	}
}

func TestClawHubClient_SearchSkills_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "key")
	_, err := c.SearchSkills(context.Background(), "anything")
	if err == nil {
		t.Fatal("expected error for bad JSON in search response")
	}
}
