// Package skill discovers and parses SKILL.md files from project and user
// directories, providing a catalog of slash-commands that can be invoked
// from the TUI.
//
// The discovery algorithm mirrors the TypeScript scanner:
//   - Max scan depth of 8 to bound symlink cycles
//   - Skips node_modules and dot-prefixed directories
//   - Alphabetical entry ordering for deterministic first-wins collision
//   - Sub-skill recursion gated by has-sub-skill: true in the parent
//   - Flat .md files at the skills-root level are treated as skills
//   - Sub-skills receive qualified names (parent.child)
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxSkillScanDepth bounds recursion so a directory symlink cycle inside
// a skill root cannot loop forever.
const maxSkillScanDepth = 8

// Skill represents a parsed SKILL.md (or flat .md) file.
type Skill struct {
	Name                     string // from frontmatter "name:", or directory/file name
	Description              string // from frontmatter "description:"
	Type                     string // "prompt", "inline", "flow", "reference" (default: activatable)
	WhenToUse                string // model-facing guidance
	HasSubSkill              bool   // opt-in for recursive sub-skill scanning
	IsSubSkill               bool   // discovered via recursion into a has-sub-skill parent
	DisableModelInvocation   bool   // only user slash activation allowed
	Source                   string // "project" or "user"
	Body                     string // markdown content after frontmatter
	Path                     string // absolute path to the SKILL.md file
}

// IsUserActivatable reports whether the skill can be invoked via slash command.
// Only types prompt, inline, flow, or empty (undefined) are user-activatable.
// Skills with type "reference" or unknown types are excluded.
func (s Skill) IsUserActivatable() bool {
	switch s.Type {
	case "", "prompt", "inline", "flow":
		return true
	default:
		return false
	}
}

// SlashName returns the TUI slash-command name for this skill.
// Sub-skills use their qualified name (e.g. "agent-core-review.slop");
// other skills use "skill:<name>" (e.g. "skill:dev-cycle").
func (s Skill) SlashName() string {
	if s.IsSubSkill {
		return s.Name
	}
	return "skill:" + s.Name
}

// Catalog holds discovered skills indexed by name.
type Catalog struct {
	skills map[string]Skill
	order  []string // insertion order for deterministic listing
}

// List returns all discovered skills in discovery order.
func (c *Catalog) List() []Skill {
	result := make([]Skill, 0, len(c.order))
	for _, name := range c.order {
		result = append(result, c.skills[name])
	}
	return result
}

// Get returns a skill by name, or nil if not found.
func (c *Catalog) Get(name string) *Skill {
	s, ok := c.skills[name]
	if !ok {
		return nil
	}
	return &s
}

// Len returns the number of discovered skills.
func (c *Catalog) Len() int { return len(c.skills) }

// Discover walks project and user directories looking for skill definitions.
// It follows the TS scanner's search root resolution:
//  1. <projectRoot>/.kimi-code/skills  (project)
//  2. <projectRoot>/.agents/skills     (project)
//  3. <userHome>/.kimi-code/skills     (user)
//  4. <userHome>/.agents/skills        (user)
//
// First-wins: when two roots define a skill with the same name, the
// first-visited root's skill is kept.
func Discover(projectRoot string) (*Catalog, error) {
	catalog := &Catalog{skills: make(map[string]Skill)}

	// Resolve project root (walk up to find .git).
	gitRoot := FindProjectRoot(projectRoot)

	var roots []skillRoot

	// Project roots.
	addRoot(&roots, filepath.Join(gitRoot, ".kimi-code", "skills"), "project")
	addRoot(&roots, filepath.Join(gitRoot, ".agents", "skills"), "project")

	// User roots.
	if home, err := os.UserHomeDir(); err == nil {
		addRoot(&roots, filepath.Join(home, ".kimi-code", "skills"), "user")
		addRoot(&roots, filepath.Join(home, ".agents", "skills"), "user")
	}

	// Deduplicate via EvalSymlinks (mirrors TS realpath).
	roots = dedupRoots(roots)

	for _, root := range roots {
		if err := walkSkillDir(root.path, root.path, root.source, 0, "", catalog); err != nil && !os.IsNotExist(err) {
			// Skip missing directories silently.
			continue
		}
	}

	return catalog, nil
}

// skillRoot pairs a directory path with its source label.
type skillRoot struct {
	path   string
	source string
}

func addRoot(roots *[]skillRoot, path, source string) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		*roots = append(*roots, skillRoot{path: path, source: source})
	}
}

func dedupRoots(roots []skillRoot) []skillRoot {
	seen := make(map[string]bool)
	var out []skillRoot
	for _, r := range roots {
		resolved, err := filepath.EvalSymlinks(r.path)
		if err != nil {
			resolved = r.path
		}
		if !seen[resolved] {
			seen[resolved] = true
			out = append(out, skillRoot{path: r.path, source: r.source})
		}
	}
	return out
}

// walkSkillDir recursively scans a directory for skill definitions.
// It mirrors the TS fileSkillDiscovery walk algorithm.
func walkSkillDir(dirPath, rootPath, source string, depth int, parentName string, catalog *Catalog) error {
	if depth > maxSkillScanDepth {
		return nil
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	// Sort entries alphabetically for deterministic ordering.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	isTopLevel := depth == 0

	// Track directory skills found at this level.
	directorySkills := make(map[string]Skill) // dirname → Skill
	// Track which directory skills allow sub-skill recursion.
	allowedSubSkillBundles := make(map[string]string) // dirname → parent skill name

	// Phase 1: discover directory skills (dirs containing SKILL.md).
	for _, entry := range entries {
		name := entry.Name()

		// Skip node_modules and dot-prefixed entries.
		if name == "node_modules" || strings.HasPrefix(name, ".") {
			continue
		}

		entryPath := filepath.Join(dirPath, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.IsDir() {
			skillMdPath := filepath.Join(entryPath, "SKILL.md")
			if isFile(skillMdPath) {
				skill, err := parseSkillFile(skillMdPath, source)
				if err == nil {
					if registerSkill(skill, catalog, parentName) {
						directorySkills[name] = skill
						if skill.HasSubSkill {
							allowedSubSkillBundles[name] = skill.Name
						}
					}
				}
			}
			continue
		}

		// Phase 2: flat .md files at top level only.
		if isTopLevel && strings.HasSuffix(name, ".md") && name != "SKILL.md" {
			skillName := strings.TrimSuffix(name, ".md")
			// Directory skills take precedence over flat files with same name.
			if _, exists := directorySkills[skillName]; exists {
				continue
			}
			skill, err := parseFlatMarkdown(filepath.Join(dirPath, name), skillName, source)
			if err == nil {
				registerSkill(skill, catalog, "")
			}
		}
	}

	// Phase 3: recurse into subdirectories.
	for _, entry := range entries {
		name := entry.Name()
		if name == "node_modules" || strings.HasPrefix(name, ".") {
			continue
		}

		entryPath := filepath.Join(dirPath, name)
		info, err := entry.Info()
		if err != nil || !info.IsDir() {
			continue
		}

		// Directory skills with SKILL.md: only recurse if has-sub-skill is true.
		if _, isDirSkill := directorySkills[name]; isDirSkill {
			if parentSkillName, allowed := allowedSubSkillBundles[name]; allowed {
				_ = walkSkillDir(entryPath, rootPath, source, depth+1, parentSkillName, catalog)
			}
			continue
		}

		// Non-skill directories: recurse normally (still within depth limit).
		// Pass "" as parent — only has-sub-skill bundles establish sub-skill lineage.
		_ = walkSkillDir(entryPath, rootPath, source, depth+1, "", catalog)
	}

	return nil
}

// registerSkill adds a skill to the catalog, applying sub-skill name qualification.
// When parentName is non-empty, the skill is a sub-skill and its name is
// qualified as "parentName.childName" (mirrors TS qualifySubSkillName).
// Returns true if the skill was newly registered (first-wins).
func registerSkill(skill Skill, catalog *Catalog, parentName string) bool {
	if parentName != "" {
		skill.IsSubSkill = true
		if skill.Name != parentName && !strings.HasPrefix(skill.Name, parentName+".") {
			skill.Name = parentName + "." + skill.Name
		}
	}
	if _, exists := catalog.skills[skill.Name]; exists {
		return false
	}
	catalog.skills[skill.Name] = skill
	catalog.order = append(catalog.order, skill.Name)
	return true
}

// parseSkillFile parses a SKILL.md file at the given path.
func parseSkillFile(path, source string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read %s: %w", path, err)
	}

	skill, err := parseFrontmatter(string(data))
	if err != nil {
		return Skill{}, err
	}

	skill.Body = readBody(string(data))
	skill.Path = path
	skill.Source = source
	return skill, nil
}

// parseFlatMarkdown parses a flat .md file as a skill.
// Name falls back to the filename (without .md); description falls back
// to the first non-empty line of the body (truncated to 240 chars).
func parseFlatMarkdown(path, fallbackName, source string) (Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, fmt.Errorf("read %s: %w", path, err)
	}

	content := string(data)
	skill, err := parseFrontmatter(content)
	if err != nil {
		// No frontmatter: use filename as name, first line as description.
		skill = Skill{Name: fallbackName}
		skill.Body = content
		skill.Description = descriptionFromBody(content)
	} else {
		skill.Body = readBody(content)
		if skill.Name == "" {
			skill.Name = fallbackName
		}
		if skill.Description == "" {
			skill.Description = descriptionFromBody(skill.Body)
		}
	}

	skill.Path = path
	skill.Source = source
	return skill, nil
}

// parseFrontmatter extracts fields from YAML frontmatter delimited by "---" markers.
// Recognized fields mirror the TS scanner: name, description, type, whenToUse,
// has-sub-skill, isSubSkill, disableModelInvocation.
func parseFrontmatter(content string) (Skill, error) {
	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return Skill{}, fmt.Errorf("no frontmatter found")
	}

	// Find the closing ---
	rest := content[3:] // skip first "---\n"
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return Skill{}, fmt.Errorf("unclosed frontmatter")
	}

	fm := rest[:endIdx]
	var s Skill

	for _, line := range strings.Split(fm, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, val, ok := splitKeyValue(line)
		if !ok {
			continue
		}
		// Strip surrounding quotes from YAML string values.
		val = stripYAMLQuotes(val)

		switch key {
		case "name":
			s.Name = val
		case "description":
			s.Description = val
		case "type":
			s.Type = val
		case "whenToUse", "when-to-use", "when_to_use":
			s.WhenToUse = val
		case "has-sub-skill", "hasSubSkill":
			s.HasSubSkill = val == "true"
		case "isSubSkill":
			s.IsSubSkill = val == "true"
		case "disableModelInvocation", "disable-model-invocation", "disable_model_invocation":
			s.DisableModelInvocation = val == "true"
		}
	}

	// SKILL.md bundles require name and description.
	if s.Name == "" && s.Description == "" {
		return Skill{}, fmt.Errorf("frontmatter missing 'name' and 'description'")
	}
	if s.Name == "" {
		return Skill{}, fmt.Errorf("frontmatter missing 'name'")
	}

	return s, nil
}

// readBody returns the markdown content after the frontmatter.
func readBody(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return content
	}
	body := rest[endIdx+4:] // skip "\n---"
	return strings.TrimLeft(body, "\n\r")
}

// descriptionFromBody returns the first non-empty line of the body,
// truncated to 240 characters (mirrors TS descriptionFromBody).
func descriptionFromBody(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 240 {
				return line[:237] + "..."
			}
			return line
		}
	}
	return ""
}

// splitKeyValue splits "key: value" into (key, value, true).
func splitKeyValue(line string) (string, string, bool) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:idx])
	val := strings.TrimSpace(line[idx+1:])
	return key, val, true
}

// stripYAMLQuotes removes surrounding double or single quotes from a value.
func stripYAMLQuotes(val string) string {
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'') {
			return val[1 : len(val)-1]
		}
	}
	return val
}

// isFile returns true if path exists and is a regular file.
func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// FindProjectRoot walks up from workDir looking for a .git directory.
// Falls back to workDir itself when no .git is found.
func FindProjectRoot(workDir string) string {
	start := workDir
	current, err := filepath.Abs(workDir)
	if err != nil {
		return start
	}
	for {
		if info, err := os.Stat(filepath.Join(current, ".git")); err == nil && info.IsDir() {
			return current
		}
		parent := filepath.Dir(current)
		if parent == current {
			return start // fallback to the original workDir
		}
		current = parent
	}
}
