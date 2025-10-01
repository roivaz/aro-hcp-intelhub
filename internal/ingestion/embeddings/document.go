package embeddings

import (
	"strings"
)

type PRDocument struct {
	Title       string
	Body        string
	Description string
}

func BuildDocument(prTitle, prBody, richDescription string) string {
	var builder strings.Builder
	builder.WriteString("PR Title: ")
	builder.WriteString(prTitle)
	builder.WriteString("\n\nPR Description: ")
	if len(prBody) > 2000 {
		builder.WriteString(prBody[:2000])
	} else {
		builder.WriteString(prBody)
	}
	if richDescription != "" {
		builder.WriteString("\n\nAI Analysis: ")
		if len(richDescription) > 3000 {
			builder.WriteString(richDescription[:3000])
		} else {
			builder.WriteString(richDescription)
		}
	}
	return builder.String()
}
