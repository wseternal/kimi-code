package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/session"
)

// ExportSession exports a session's conversation as markdown.
func ExportSession(store *session.SessionStore, sessionID string, output string) error {
	ctx := context.Background()

	// Load session metadata
	data, err := store.Persist().Load(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}

	// Load messages
	if err := store.History().Load(ctx, sessionID); err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	turns := store.History().Turns()

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", data.Title))
	b.WriteString(fmt.Sprintf("**Session:** %s  \n", data.ID))
	b.WriteString(fmt.Sprintf("**Created:** %s  \n", data.CreatedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("**Updated:** %s  \n\n", data.UpdatedAt.Format(time.RFC3339)))
	b.WriteString("---\n\n")

	for i, turn := range turns {
		b.WriteString(fmt.Sprintf("## Turn %d\n\n", i+1))
		b.WriteString(fmt.Sprintf("### User\n\n%s\n\n", turn.Prompt))

		if turn.Thinking != "" {
			b.WriteString("<details>\n<summary>Thinking</summary>\n\n")
			b.WriteString(turn.Thinking)
			b.WriteString("\n\n</details>\n\n")
		}

		if len(turn.Tools) > 0 {
			b.WriteString("<details>\n<summary>Tools</summary>\n\n")
			for _, tool := range turn.Tools {
				b.WriteString(fmt.Sprintf("- **%s**", tool.Name))
				if tool.IsError {
					b.WriteString(" (error)")
				}
				b.WriteString("\n")
			}
			b.WriteString("\n</details>\n\n")
		}

		b.WriteString(fmt.Sprintf("### Assistant\n\n%s\n\n", turn.Response))
	}

	content := b.String()

	if output == "" || output == "-" {
		fmt.Print(content)
		return nil
	}

	return os.WriteFile(output, []byte(content), 0644)
}
