// Package clawhub provides a client for the ClawHub skill marketplace at clawhub.com.
// It supports listing, searching, publishing, and syncing skills to local directories.
package clawhub

import "time"

// SkillMeta contains metadata about a skill in the ClawHub marketplace.
type SkillMeta struct {
	// ID is the unique skill identifier (e.g. "weather-v2").
	ID string `json:"id"`
	// Name is the human-readable skill name.
	Name string `json:"name"`
	// Version is the semantic version string (e.g. "1.2.0").
	Version string `json:"version"`
	// Description is a short description of what the skill does.
	Description string `json:"description"`
	// Author is the skill author's name or identifier.
	Author string `json:"author"`
	// Tags are searchable keywords for the skill.
	Tags []string `json:"tags"`
	// Downloads is the total download count.
	Downloads int64 `json:"downloads"`
	// Stars is the number of marketplace stars/upvotes.
	Stars int `json:"stars"`
	// CreatedAt is when the skill was first published.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the skill was last updated.
	UpdatedAt time.Time `json:"updated_at"`
	// License is the SPDX license identifier (e.g. "MIT").
	License string `json:"license"`
}

// Skill is a fully loaded skill including its manifest and file contents.
type Skill struct {
	SkillMeta
	// Files maps relative file paths to their content.
	// e.g. "SKILL.md" → "<markdown content>"
	Files map[string]string `json:"files"`
	// Manifest is the parsed SKILL.md frontmatter.
	Manifest map[string]interface{} `json:"manifest"`
	// Readme is the full SKILL.md content.
	Readme string `json:"readme"`
	// Checksum is the SHA-256 checksum of the skill archive.
	Checksum string `json:"checksum"`
}

// SkillFilter defines search/filter criteria for listing skills.
type SkillFilter struct {
	// Tags filters by one or more tags (AND logic).
	Tags []string `json:"tags,omitempty"`
	// Author filters by author name.
	Author string `json:"author,omitempty"`
	// MinStars filters to skills with at least this many stars.
	MinStars int `json:"min_stars,omitempty"`
	// Query is a free-text search query.
	Query string `json:"query,omitempty"`
	// Limit is the maximum number of results to return (default 20, max 100).
	Limit int `json:"limit,omitempty"`
	// Offset is the pagination offset.
	Offset int `json:"offset,omitempty"`
	// SortBy controls sort order: "downloads", "stars", "updated", "created".
	SortBy string `json:"sort_by,omitempty"`
}

// PublishRequest is sent to the ClawHub API when publishing a skill.
type PublishRequest struct {
	Skill Skill `json:"skill"`
}

// PublishResponse is returned by the ClawHub API after a successful publish.
type PublishResponse struct {
	// ID is the assigned skill ID (may differ from requested ID if taken).
	ID string `json:"id"`
	// Version is the assigned version.
	Version string `json:"version"`
	// URL is the canonical marketplace URL for the skill.
	URL string `json:"url"`
}

// apiListResponse wraps the paginated list API response.
type apiListResponse struct {
	Skills []SkillMeta `json:"skills"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}
