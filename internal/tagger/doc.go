package tagger

import (
	"sort"
	"strings"

	"github.com/RenlySir/kb-tool/internal/source"
)

type RuleBasedTagger struct{}

func NewRuleBasedTagger() RuleBasedTagger {
	return RuleBasedTagger{}
}

func (RuleBasedTagger) Tag(doc source.Document) []string {
	text := strings.ToLower(doc.Path + "\n" + doc.Title + "\n" + doc.Language + "\n" + doc.Content)
	tags := map[string]struct{}{}

	add := func(tag string) {
		if tag != "" {
			tags[tag] = struct{}{}
		}
	}

	add(normalizeTag(doc.Language))

	rules := map[string][]string{
		"backend":        {"package ", "internal/", "cmd/", "api", "server", "service", "handler", "store"},
		"database":       {"database", "db", "sql", "mysql", "tidb", "postgres", "schema", "storage", "store"},
		"documentation":  {"readme", ".md", "docs/", "documentation"},
		"github":         {"github", "github.com"},
		"gitlab":         {"gitlab", "gitlab.com"},
		"knowledge-base": {"knowledge base", "knowledge-base", "kb_", "kb tool", "kb-tool"},
		"mysql":          {"mysql", "mysql-compatible", "mysql compatible"},
		"tidb":           {"tidb"},
		"cli":            {"cobra", "flag.", "command", "cli"},
		"devops":         {"docker", "kubernetes", "helm", "terraform", "ci", "workflow"},
	}

	for tag, keywords := range rules {
		for _, keyword := range keywords {
			if strings.Contains(text, keyword) {
				add(tag)
				break
			}
		}
	}

	out := make([]string, 0, len(tags))
	for tag := range tags {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(strings.ToLower(tag))
	tag = strings.ReplaceAll(tag, "_", "-")
	return tag
}
