package orchestrator

import "strings"

// ContentKind identifies the type of a ContentBlock.
type ContentKind string

const (
	ContentKindText  ContentKind = "text"
	ContentKindImage ContentKind = "image"
	ContentKindError ContentKind = "error"
)

// ContentBlock is a single unit of tool output — text, image, or error.
// Replaces bare string returns for richer multimodal support.
type ContentBlock struct {
	Kind ContentKind `json:"kind"`
	Text string      `json:"text,omitempty"`

	// Image fields (base64 encoded, for vision-capable models)
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"` // base64

	// Error fields
	ErrCode string `json:"err_code,omitempty"`
}

// TextBlock creates a text ContentBlock.
func TextBlock(text string) ContentBlock {
	return ContentBlock{
		Kind: ContentKindText,
		Text: text,
	}
}

// ImageBlock creates an image ContentBlock.
func ImageBlock(mimeType, base64Data string) ContentBlock {
	return ContentBlock{
		Kind:     ContentKindImage,
		MimeType: mimeType,
		Data:     base64Data,
	}
}

// ErrorBlock creates an error ContentBlock.
func ErrorBlock(errCode, message string) ContentBlock {
	return ContentBlock{
		Kind:    ContentKindError,
		Text:    message,
		ErrCode: errCode,
	}
}

// ToolOutput is the structured return value from a tool execution.
type ToolOutput struct {
	Content   []ContentBlock `json:"content"`
	ElapsedMs int64          `json:"elapsed_ms"`
	ExitCode  int            `json:"exit_code,omitempty"`
}

// Text returns all text content blocks concatenated with newlines.
func (o *ToolOutput) Text() string {
	var parts []string
	for _, block := range o.Content {
		if block.Kind == ContentKindText && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// HasError returns true if any content block is an error.
func (o *ToolOutput) HasError() bool {
	for _, block := range o.Content {
		if block.Kind == ContentKindError {
			return true
		}
	}
	return false
}

// ToLegacyResult converts to the legacy ToolResult format for backward compatibility
// with the existing ToolLoop and MQTT-based edge agent flow.
func (o *ToolOutput) ToLegacyResult(toolName string) ToolResult {
	status := "success"
	errMsg := ""
	errType := ""
	result := o.Text()

	if o.HasError() {
		status = "error"
		// Collect error messages
		var errs []string
		for _, block := range o.Content {
			if block.Kind == ContentKindError {
				errs = append(errs, block.Text)
				if errType == "" {
					errType = block.ErrCode
				}
			}
		}
		errMsg = strings.Join(errs, "; ")
		if result == "" {
			result = errMsg
		}
	}

	return ToolResult{
		Tool:      toolName,
		Status:    status,
		Result:    result,
		Error:     errMsg,
		ErrorType: errType,
		ElapsedMs: o.ElapsedMs,
		ExitCode:  o.ExitCode,
	}
}
