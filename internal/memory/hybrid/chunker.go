package hybrid

import (
	"strings"
)

// Chunk represents a text chunk with its section context.
type Chunk struct {
	Text    string // The chunk content.
	Heading string // The most recent heading before this chunk.
	Index   int    // Chunk index within the document.
}

// ChunkText splits markdown text into overlapping chunks, preserving heading context.
func ChunkText(text string, chunkSize, overlap int) []Chunk {
	if chunkSize <= 0 {
		chunkSize = 512
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = 0
	}
	if len(text) == 0 {
		return nil
	}

	lines := strings.Split(text, "\n")
	var chunks []Chunk
	currentHeading := ""
	var buf strings.Builder
	idx := 0

	flush := func() {
		content := strings.TrimSpace(buf.String())
		if content != "" {
			chunks = append(chunks, Chunk{
				Text:    content,
				Heading: currentHeading,
				Index:   idx,
			})
			idx++
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			// Flush current buffer before heading change
			flush()
			currentHeading = trimmed
			buf.Reset()
			buf.WriteString(line)
			buf.WriteString("\n")
			continue
		}

		if buf.Len()+len(line)+1 > chunkSize && buf.Len() > 0 {
			flush()
			// Apply overlap: keep last `overlap` chars
			prev := buf.String()
			buf.Reset()
			if overlap > 0 && len(prev) > overlap {
				buf.WriteString(prev[len(prev)-overlap:])
			}
		}
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	flush()
	return chunks
}
