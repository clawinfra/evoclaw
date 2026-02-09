package memory

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	MaxHotSizeBytes       = 5120 // 5KB hard cap
	MaxCriticalLessons    = 20
	MaxActiveProjects     = 5
	MaxActiveTasks        = 10
)

// HotMemory represents core memory (Tier 1) — always in context
type HotMemory struct {
	Identity       IdentityInfo       `json:"identity"`
	OwnerProfile   OwnerProfile       `json:"owner_profile"`
	ActiveContext  ActiveContext      `json:"active_context"`
	CriticalLessons []Lesson          `json:"critical_lessons"`
	Version        int                `json:"version"`
	LastUpdated    time.Time          `json:"last_updated"`
}

// IdentityInfo contains core agent identity
type IdentityInfo struct {
	AgentName           string    `json:"agent_name"`
	OwnerName           string    `json:"owner_name"`
	OwnerPreferredName  string    `json:"owner_preferred_name,omitempty"`
	RelationshipStart   time.Time `json:"relationship_start"`
	TrustLevel          float64   `json:"trust_level"` // 0-1
}

// OwnerProfile contains learned information about the owner
type OwnerProfile struct {
	Personality  string            `json:"personality,omitempty"`
	Family       []string          `json:"family,omitempty"`
	TopicsLoved  []string          `json:"topics_loved,omitempty"`
	TopicsAvoid  []string          `json:"topics_avoid,omitempty"`
	Preferences  map[string]string `json:"preferences,omitempty"`
	Schedule     map[string]string `json:"schedule,omitempty"` // e.g., "morning_mood": "cheerful"
}

// ActiveContext is rewritten after every conversation
type ActiveContext struct {
	CurrentProjects []Project `json:"current_projects,omitempty"`
	RecentEvents    []Event   `json:"recent_events,omitempty"`
	PendingTasks    []Task    `json:"pending_tasks,omitempty"`
}

// Project represents an active project
type Project struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	StartDate   time.Time `json:"start_date"`
	Status      string    `json:"status,omitempty"`
}

// Event represents a recent event
type Event struct {
	Description string    `json:"description"`
	Date        time.Time `json:"date"`
}

// Task represents a pending action item
type Task struct {
	Description string     `json:"description"`
	DueDate     *time.Time `json:"due_date,omitempty"`
	Priority    string     `json:"priority,omitempty"` // "high", "medium", "low"
}

// Lesson represents a critical lesson learned
type Lesson struct {
	Text       string    `json:"text"`
	Importance float64   `json:"importance"` // 0-1
	LearnedAt  time.Time `json:"learned_at"`
	Category   string    `json:"category,omitempty"` // e.g., "communication", "preferences"
}

// NewHotMemory creates a new empty hot memory
func NewHotMemory(agentName, ownerName string) *HotMemory {
	return &HotMemory{
		Identity: IdentityInfo{
			AgentName:         agentName,
			OwnerName:         ownerName,
			RelationshipStart: time.Now(),
			TrustLevel:        0.5,
		},
		OwnerProfile: OwnerProfile{
			Preferences: make(map[string]string),
			Schedule:    make(map[string]string),
		},
		ActiveContext: ActiveContext{
			CurrentProjects: make([]Project, 0),
			RecentEvents:    make([]Event, 0),
			PendingTasks:    make([]Task, 0),
		},
		CriticalLessons: make([]Lesson, 0),
		Version:         1,
		LastUpdated:     time.Now(),
	}
}

// UpdateIdentity updates identity information
func (h *HotMemory) UpdateIdentity(preferredName *string, trustLevel *float64) error {
	if preferredName != nil {
		h.Identity.OwnerPreferredName = *preferredName
	}
	if trustLevel != nil {
		if *trustLevel < 0 || *trustLevel > 1 {
			return fmt.Errorf("trust level must be between 0 and 1")
		}
		h.Identity.TrustLevel = *trustLevel
	}

	h.Version++
	h.LastUpdated = time.Now()
	return h.enforceSize()
}

// UpdateProfile updates owner profile
func (h *HotMemory) UpdateProfile(personality *string, family, topicsLoved, topicsAvoid *[]string) error {
	if personality != nil {
		h.OwnerProfile.Personality = *personality
	}
	if family != nil {
		h.OwnerProfile.Family = *family
	}
	if topicsLoved != nil {
		h.OwnerProfile.TopicsLoved = *topicsLoved
	}
	if topicsAvoid != nil {
		h.OwnerProfile.TopicsAvoid = *topicsAvoid
	}

	h.Version++
	h.LastUpdated = time.Now()
	return h.enforceSize()
}

// AddPreference adds or updates a preference
func (h *HotMemory) AddPreference(key, value string) error {
	h.OwnerProfile.Preferences[key] = value
	h.Version++
	h.LastUpdated = time.Now()
	return h.enforceSize()
}

// AddProject adds a new active project
func (h *HotMemory) AddProject(project Project) error {
	// Check limit
	if len(h.ActiveContext.CurrentProjects) >= MaxActiveProjects {
		return fmt.Errorf("max %d active projects reached", MaxActiveProjects)
	}

	h.ActiveContext.CurrentProjects = append(h.ActiveContext.CurrentProjects, project)
	h.Version++
	h.LastUpdated = time.Now()
	return h.enforceSize()
}

// RemoveProject removes a project by name
func (h *HotMemory) RemoveProject(name string) error {
	for i, proj := range h.ActiveContext.CurrentProjects {
		if proj.Name == name {
			h.ActiveContext.CurrentProjects = append(
				h.ActiveContext.CurrentProjects[:i],
				h.ActiveContext.CurrentProjects[i+1:]...,
			)
			h.Version++
			h.LastUpdated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("project %s not found", name)
}

// AddEvent adds a recent event
func (h *HotMemory) AddEvent(event Event) error {
	h.ActiveContext.RecentEvents = append(h.ActiveContext.RecentEvents, event)

	// Keep only last 10 events
	if len(h.ActiveContext.RecentEvents) > 10 {
		h.ActiveContext.RecentEvents = h.ActiveContext.RecentEvents[len(h.ActiveContext.RecentEvents)-10:]
	}

	h.Version++
	h.LastUpdated = time.Now()
	return h.enforceSize()
}

// AddTask adds a pending task
func (h *HotMemory) AddTask(task Task) error {
	if len(h.ActiveContext.PendingTasks) >= MaxActiveTasks {
		return fmt.Errorf("max %d pending tasks reached", MaxActiveTasks)
	}

	h.ActiveContext.PendingTasks = append(h.ActiveContext.PendingTasks, task)
	h.Version++
	h.LastUpdated = time.Now()
	return h.enforceSize()
}

// RemoveTask removes a task by description
func (h *HotMemory) RemoveTask(description string) error {
	for i, task := range h.ActiveContext.PendingTasks {
		if task.Description == description {
			h.ActiveContext.PendingTasks = append(
				h.ActiveContext.PendingTasks[:i],
				h.ActiveContext.PendingTasks[i+1:]...,
			)
			h.Version++
			h.LastUpdated = time.Now()
			return nil
		}
	}
	return fmt.Errorf("task not found")
}

// AddLesson adds a critical lesson
func (h *HotMemory) AddLesson(lesson Lesson) error {
	// If at capacity, remove lowest-scored lesson
	if len(h.CriticalLessons) >= MaxCriticalLessons {
		if err := h.pruneLesson(); err != nil {
			return err
		}
	}

	h.CriticalLessons = append(h.CriticalLessons, lesson)
	h.Version++
	h.LastUpdated = time.Now()
	return h.enforceSize()
}

// pruneLesson removes the lowest-importance lesson
func (h *HotMemory) pruneLesson() error {
	if len(h.CriticalLessons) == 0 {
		return nil
	}

	// Find lesson with lowest importance
	minIdx := 0
	minImportance := h.CriticalLessons[0].Importance

	for i, lesson := range h.CriticalLessons {
		if lesson.Importance < minImportance {
			minIdx = i
			minImportance = lesson.Importance
		}
	}

	// Remove it
	h.CriticalLessons = append(
		h.CriticalLessons[:minIdx],
		h.CriticalLessons[minIdx+1:]...,
	)

	return nil
}

// Serialize converts hot memory to JSON
func (h *HotMemory) Serialize() ([]byte, error) {
	return json.Marshal(h)
}

// DeserializeHotMemory loads hot memory from JSON
func DeserializeHotMemory(data []byte) (*HotMemory, error) {
	var h HotMemory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("unmarshal hot memory: %w", err)
	}
	return &h, nil
}

// enforceSize ensures hot memory stays under size limit
func (h *HotMemory) enforceSize() error {
	data, err := h.Serialize()
	if err != nil {
		return err
	}

	if len(data) <= MaxHotSizeBytes {
		return nil
	}

	// Exceed size limit — prune aggressively
	for len(data) > MaxHotSizeBytes && len(h.CriticalLessons) > 0 {
		// Remove least important lesson
		if err := h.pruneLesson(); err != nil {
			return err
		}
		data, err = h.Serialize()
		if err != nil {
			return err
		}
	}

	// If still too large, prune old events
	for len(data) > MaxHotSizeBytes && len(h.ActiveContext.RecentEvents) > 0 {
		h.ActiveContext.RecentEvents = h.ActiveContext.RecentEvents[1:]
		data, err = h.Serialize()
		if err != nil {
			return err
		}
	}

	// If still too large, prune completed tasks
	for len(data) > MaxHotSizeBytes && len(h.ActiveContext.PendingTasks) > 0 {
		h.ActiveContext.PendingTasks = h.ActiveContext.PendingTasks[1:]
		data, err = h.Serialize()
		if err != nil {
			return err
		}
	}

	if len(data) > MaxHotSizeBytes {
		return fmt.Errorf("hot memory too large (%d bytes) even after pruning", len(data))
	}

	return nil
}

// GetSize returns the current serialized size in bytes
func (h *HotMemory) GetSize() (int, error) {
	data, err := h.Serialize()
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// ClearActiveContext resets active context (useful for daily refresh)
func (h *HotMemory) ClearActiveContext() {
	h.ActiveContext = ActiveContext{
		CurrentProjects: h.ActiveContext.CurrentProjects, // Keep projects
		RecentEvents:    make([]Event, 0),
		PendingTasks:    make([]Task, 0),
	}
	h.Version++
	h.LastUpdated = time.Now()
}
