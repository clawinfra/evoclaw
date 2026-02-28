package clawhub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the production ClawHub API endpoint.
	DefaultBaseURL = "https://api.clawhub.com/v1"

	// defaultTimeout is the HTTP client timeout.
	defaultTimeout = 30 * time.Second
)

// Client is an HTTP client for the ClawHub skill marketplace API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new ClawHub API client.
// baseURL should be the API base (e.g. "https://api.clawhub.com/v1").
// apiKey is required for write operations (publish); read operations may work without it.
func NewClient(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// ListSkills lists skills from the marketplace with optional filtering.
func (c *Client) ListSkills(ctx context.Context, filter SkillFilter) ([]SkillMeta, error) {
	params := url.Values{}
	if filter.Author != "" {
		params.Set("author", filter.Author)
	}
	if filter.MinStars > 0 {
		params.Set("min_stars", strconv.Itoa(filter.MinStars))
	}
	if filter.Limit > 0 {
		params.Set("limit", strconv.Itoa(filter.Limit))
	}
	if filter.Offset > 0 {
		params.Set("offset", strconv.Itoa(filter.Offset))
	}
	if filter.SortBy != "" {
		params.Set("sort_by", filter.SortBy)
	}
	for _, tag := range filter.Tags {
		params.Add("tag", tag)
	}

	endpoint := c.baseURL + "/skills"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	var resp apiListResponse
	if err := c.get(ctx, endpoint, &resp); err != nil {
		return nil, fmt.Errorf("clawhub: list skills: %w", err)
	}

	return resp.Skills, nil
}

// GetSkill fetches a single skill by ID.
func (c *Client) GetSkill(ctx context.Context, id string) (*Skill, error) {
	if id == "" {
		return nil, fmt.Errorf("clawhub: skill ID is required")
	}

	var skill Skill
	if err := c.get(ctx, c.baseURL+"/skills/"+url.PathEscape(id), &skill); err != nil {
		return nil, fmt.Errorf("clawhub: get skill %q: %w", id, err)
	}

	return &skill, nil
}

// PublishSkill publishes or updates a skill in the ClawHub marketplace.
// Requires a valid API key.
func (c *Client) PublishSkill(ctx context.Context, skill Skill) (*PublishResponse, error) {
	if skill.ID == "" {
		return nil, fmt.Errorf("clawhub: skill ID is required")
	}
	if skill.Name == "" {
		return nil, fmt.Errorf("clawhub: skill Name is required")
	}
	if c.apiKey == "" {
		return nil, fmt.Errorf("clawhub: API key required for publish")
	}

	body, err := json.Marshal(PublishRequest{Skill: skill})
	if err != nil {
		return nil, fmt.Errorf("clawhub: marshal publish request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/skills", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("clawhub: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("clawhub: publish request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	if err := checkStatus(httpResp); err != nil {
		return nil, fmt.Errorf("clawhub: publish skill: %w", err)
	}

	var pubResp PublishResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&pubResp); err != nil {
		return nil, fmt.Errorf("clawhub: decode publish response: %w", err)
	}

	return &pubResp, nil
}

// SearchSkills searches for skills matching the given query string.
func (c *Client) SearchSkills(ctx context.Context, query string) ([]SkillMeta, error) {
	if query == "" {
		return nil, fmt.Errorf("clawhub: search query is required")
	}

	endpoint := c.baseURL + "/skills/search?q=" + url.QueryEscape(query)

	var resp apiListResponse
	if err := c.get(ctx, endpoint, &resp); err != nil {
		return nil, fmt.Errorf("clawhub: search skills: %w", err)
	}

	return resp.Skills, nil
}

// get performs an authenticated GET request and decodes the JSON response.
func (c *Client) get(ctx context.Context, endpoint string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := checkStatus(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

// checkStatus returns an error if the HTTP response status indicates failure.
func checkStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
