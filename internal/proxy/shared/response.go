package shared

import "strings"

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func ChunkText(text string, chunkSize int) []string {
	if chunkSize <= 0 || len(text) <= chunkSize {
		if strings.TrimSpace(text)=="" { return nil }
		return []string{text}
	}
	chunks:=make([]string,0,(len(text)+chunkSize-1)/chunkSize)
	for len(text)>0 {
		if len(text)<=chunkSize { chunks=append(chunks,text); break }
		chunks=append(chunks,text[:chunkSize]); text=text[chunkSize:]
	}
	return chunks
}
