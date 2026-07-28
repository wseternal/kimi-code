package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    Skill
		wantErr bool
	}{
		{
			name: "basic skill with name and description",
			content: "---\nname: gen-changesets\ndescription: Use when generating changesets.\n---\n\n# Body content here",
			want: Skill{
				Name:        "gen-changesets",
				Description: "Use when generating changesets.",
			},
		},
		{
			name: "skill with has-sub-skill",
			content: "---\nname: agent-core-review\ndescription: Code review guidance.\nhas-sub-skill: true\n---",
			want: Skill{
				Name:        "agent-core-review",
				Description: "Code review guidance.",
				HasSubSkill: true,
			},
		},
		{
			name: "skill with type field",
			content: "---\nname: ref-skill\ndescription: Reference only.\ntype: reference\n---",
			want: Skill{
				Name:        "ref-skill",
				Description: "Reference only.",
				Type:        "reference",
			},
		},
		{
			name: "skill with when-to-use alias",
			content: "---\nname: dev-cycle\ndescription: Full pipeline.\nwhen-to-use: When building features.\n---",
			want: Skill{
				Name:        "dev-cycle",
				Description: "Full pipeline.",
				WhenToUse:   "When building features.",
			},
		},
		{
			name: "skill with whenToUse camelCase",
			content: "---\nname: test\ndescription: Test.\nwhenToUse: During testing.\n---",
			want: Skill{
				Name:        "test",
				Description: "Test.",
				WhenToUse:   "During testing.",
			},
		},
		{
			name: "skill with isSubSkill",
			content: "---\nname: slop\ndescription: Sub review.\nisSubSkill: true\n---",
			want: Skill{
				Name:        "slop",
				Description: "Sub review.",
				IsSubSkill:  true,
			},
		},
		{
			name:    "no frontmatter",
			content: "# Just markdown, no frontmatter",
			want:    Skill{},
			wantErr: true,
		},
		{
			name:    "empty content",
			content: "",
			want:    Skill{},
			wantErr: true,
		},
		{
			name: "description with colons and special chars",
			content: "---\nname: dev-cycle\ndescription: Use when building a feature — plan, implement, review, simplify.\n---",
			want: Skill{
				Name:        "dev-cycle",
				Description: "Use when building a feature — plan, implement, review, simplify.",
			},
		},
		{
			name: "quoted description value",
			content: "---\nname: test\ndescription: \"A quoted description\"\n---",
			want: Skill{
				Name:        "test",
				Description: "A quoted description",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseFrontmatter(tc.content)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tc.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tc.want.Name)
			}
			if got.Description != tc.want.Description {
				t.Errorf("Description = %q, want %q", got.Description, tc.want.Description)
			}
			if got.HasSubSkill != tc.want.HasSubSkill {
				t.Errorf("HasSubSkill = %v, want %v", got.HasSubSkill, tc.want.HasSubSkill)
			}
			if got.Type != tc.want.Type {
				t.Errorf("Type = %q, want %q", got.Type, tc.want.Type)
			}
			if got.WhenToUse != tc.want.WhenToUse {
				t.Errorf("WhenToUse = %q, want %q", got.WhenToUse, tc.want.WhenToUse)
			}
			if got.IsSubSkill != tc.want.IsSubSkill {
				t.Errorf("IsSubSkill = %v, want %v", got.IsSubSkill, tc.want.IsSubSkill)
			}
		})
	}
}

func TestReadBody(t *testing.T) {
	content := "---\nname: test-skill\ndescription: A test skill.\n---\n\n# Skill Instructions\n\nDo something useful."

	body := readBody(content)
	if body == "" {
		t.Fatal("expected non-empty body")
	}
	if !strings.Contains(body, "# Skill Instructions") {
		t.Errorf("body should contain heading, got %q", body)
	}
	if strings.Contains(body, "---") {
		t.Errorf("body should not contain frontmatter delimiters")
	}
}

func TestFindProjectRoot(t *testing.T) {
	// Create a temp directory with a .git folder and a subdirectory.
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	subDir := filepath.Join(tmpDir, "src", "deep")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// From subdirectory, should find tmpDir.
	root := FindProjectRoot(subDir)
	if root != tmpDir {
		t.Errorf("FindProjectRoot(%q) = %q, want %q", subDir, root, tmpDir)
	}

	// From tmpDir itself, should find tmpDir.
	root = FindProjectRoot(tmpDir)
	if root != tmpDir {
		t.Errorf("FindProjectRoot(%q) = %q, want %q", tmpDir, root, tmpDir)
	}

	// From a directory with no .git, should fallback to the dir itself.
	noGit := t.TempDir()
	root = FindProjectRoot(noGit)
	if root != noGit {
		t.Errorf("FindProjectRoot(%q) = %q, want fallback %q", noGit, root, noGit)
	}
}

func TestDiscover(t *testing.T) {
	// Isolate from real user home.
	t.Setenv("HOME", t.TempDir())
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".agents", "skills")

	// Create skill directories with SKILL.md files.
	writeSkill(t, skillsDir, "skill-a", "---\nname: skill-a\ndescription: First skill.\n---\nBody A")
	writeSkill(t, skillsDir, "skill-b", "---\nname: skill-b\ndescription: Second skill.\n---\nBody B")

	// skill-c has has-sub-skill: true, so its child should be discovered as a sub-skill.
	writeSkill(t, skillsDir, "skill-c", "---\nname: skill-c\ndescription: Parent skill.\nhas-sub-skill: true\n---\nBody C")
	writeSkill(t, filepath.Join(skillsDir, "skill-c"), "sub-skill", "---\nname: sub-skill\ndescription: A sub-skill.\n---\nSub body")

	// skill-d does NOT have has-sub-skill, so its child should NOT be discovered.
	writeSkill(t, skillsDir, "skill-d", "---\nname: skill-d\ndescription: No sub-skill scanning.\n---\nBody D")
	writeSkill(t, filepath.Join(skillsDir, "skill-d"), "hidden-child", "---\nname: hidden-child\ndescription: Should not be found.\n---\nHidden")

	// A directory without SKILL.md should be ignored.
	otherDir := filepath.Join(skillsDir, "no-skill-here")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "README.md"), []byte("not a skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := Discover(tmpDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	all := catalog.List()
	// Should find: skill-a, skill-b, skill-c, skill-c.sub-skill, skill-d = 5 skills
	// hidden-child should NOT be found (skill-d has no has-sub-skill).
	if len(all) != 5 {
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Name
		}
		t.Fatalf("expected 5 skills, got %d: %v", len(all), names)
	}

	if s := catalog.Get("skill-a"); s == nil {
		t.Error("expected to find skill-a")
	}
	if s := catalog.Get("skill-c"); s == nil {
		t.Error("expected to find skill-c")
	}
	if s := catalog.Get("skill-c.sub-skill"); s == nil {
		t.Error("expected to find skill-c.sub-skill (qualified name)")
	} else if !s.IsSubSkill {
		t.Error("skill-c.sub-skill should have IsSubSkill=true")
	}
	if s := catalog.Get("hidden-child"); s != nil {
		t.Error("hidden-child should NOT be discovered (parent lacks has-sub-skill)")
	}
	if s := catalog.Get("skill-d.hidden-child"); s != nil {
		t.Error("skill-d.hidden-child should NOT be discovered (parent lacks has-sub-skill)")
	}
	if s := catalog.Get("nonexistent"); s != nil {
		t.Errorf("expected nil for nonexistent skill, got %+v", s)
	}
}

func TestDiscover_FlatMarkdown(t *testing.T) {
	// Flat .md files at the top level of a skills root should be discovered.
	// Isolate from real user home.
	t.Setenv("HOME", t.TempDir())
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".agents", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A flat .md with frontmatter at the skills root.
	if err := os.WriteFile(filepath.Join(skillsDir, "flat-skill.md"), []byte("---\nname: flat-skill\ndescription: A flat skill.\n---\nFlat body"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A flat .md without frontmatter — should use filename as name.
	if err := os.WriteFile(filepath.Join(skillsDir, "simple.md"), []byte("Just some instructions.\nMore lines."), 0o644); err != nil {
		t.Fatal(err)
	}

	// A nested .md should NOT be a flat skill.
	nested := filepath.Join(skillsDir, "bundle")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "helper.md"), []byte("---\nname: helper\ndescription: Nested.\n---\nNested"), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := Discover(tmpDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Should find flat-skill and simple (both at top level).
	// Nested helper.md should NOT be found (not a SKILL.md, not at top level).
	all := catalog.List()
	if len(all) != 2 {
		names := make([]string, len(all))
		for i, s := range all {
			names[i] = s.Name
		}
		t.Fatalf("expected 2 skills, got %d: %v", len(all), names)
	}

	if s := catalog.Get("flat-skill"); s == nil {
		t.Error("expected to find flat-skill")
	}
	if s := catalog.Get("simple"); s == nil {
		t.Error("expected to find simple (name from filename)")
	}
}

func TestDiscover_TypeFiltering(t *testing.T) {
	// Skills with type=reference should still be discovered but marked.
	// Isolate from real user home.
	t.Setenv("HOME", t.TempDir())
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".agents", "skills")
	writeSkill(t, skillsDir, "ref-only", "---\nname: ref-only\ndescription: Reference doc.\ntype: reference\n---\nRef body")
	writeSkill(t, skillsDir, "prompt-type", "---\nname: prompt-type\ndescription: Prompt skill.\ntype: prompt\n---\nPrompt body")

	catalog, err := Discover(tmpDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if s := catalog.Get("ref-only"); s == nil {
		t.Error("expected to find ref-only (discovered, but not user-activatable)")
	} else if s.IsUserActivatable() {
		t.Error("ref-only should NOT be user-activatable")
	}
	if s := catalog.Get("prompt-type"); s == nil {
		t.Error("expected to find prompt-type")
	} else if !s.IsUserActivatable() {
		t.Error("prompt-type should be user-activatable")
	}
}

func TestDiscover_NoSkillsDir(t *testing.T) {
	// Isolate from real user home.
	t.Setenv("HOME", t.TempDir())
	tmpDir := t.TempDir()
	catalog, err := Discover(tmpDir)
	if err != nil {
		t.Fatalf("Discover should not error for missing dir: %v", err)
	}
	if catalog.Len() != 0 {
		t.Errorf("expected 0 skills, got %d", catalog.Len())
	}
}

func TestDiscover_SkipsNodeModulesAndDotDirs(t *testing.T) {
	// Isolate from real user home.
	t.Setenv("HOME", t.TempDir())
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".agents", "skills")

	// Skill inside node_modules should be skipped.
	nmDir := filepath.Join(skillsDir, "node_modules", "bad-skill")
	writeSkill(t, nmDir, "", "---\nname: bad-skill\ndescription: Should not be found.\n---\nBad")

	// Skill inside a dot-directory should be skipped.
	dotDir := filepath.Join(skillsDir, ".hidden", "secret-skill")
	writeSkill(t, dotDir, "", "---\nname: secret-skill\ndescription: Should not be found.\n---\nSecret")

	catalog, err := Discover(tmpDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if catalog.Len() != 0 {
		t.Errorf("expected 0 skills (node_modules and dot-dirs skipped), got %d", catalog.Len())
	}
}

func TestDiscover_UserHome(t *testing.T) {
	// Test user home skill discovery with a controlled HOME.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	userSkillsDir := filepath.Join(tmpHome, ".agents", "skills")
	writeSkill(t, userSkillsDir, "user-skill", "---\nname: user-skill\ndescription: User home skill.\n---\nUser body")

	// Also create project dir (empty, no skills).
	projectDir := t.TempDir()

	catalog, err := Discover(projectDir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if s := catalog.Get("user-skill"); s == nil {
		t.Error("expected to find user-skill from user home")
	} else if s.Source != "user" {
		t.Errorf("user-skill Source = %q, want 'user'", s.Source)
	}
}

func TestSkillSlashName(t *testing.T) {
	tests := []struct {
		skill    Skill
		wantName string
	}{
		{Skill{Name: "gen-changesets", Source: "project"}, "skill:gen-changesets"},
		{Skill{Name: "skill-c.sub-skill", IsSubSkill: true, Source: "project"}, "skill-c.sub-skill"},
		{Skill{Name: "user-skill", Source: "user"}, "skill:user-skill"},
	}
	for _, tc := range tests {
		got := tc.skill.SlashName()
		if got != tc.wantName {
			t.Errorf("Skill(%q, sub=%v, src=%q).SlashName() = %q, want %q",
				tc.skill.Name, tc.skill.IsSubSkill, tc.skill.Source, got, tc.wantName)
		}
	}
}

func TestSkillIsUserActivatable(t *testing.T) {
	tests := []struct {
		typeVal string
		want    bool
	}{
		{"", true},         // undefined type is activatable
		{"prompt", true},   // prompt is activatable
		{"inline", true},   // inline is activatable
		{"flow", true},     // flow is activatable
		{"reference", false}, // reference is NOT activatable
		{"unknown", false},   // unknown type is NOT activatable
	}
	for _, tc := range tests {
		s := Skill{Type: tc.typeVal}
		if got := s.IsUserActivatable(); got != tc.want {
			t.Errorf("Skill{Type: %q}.IsUserActivatable() = %v, want %v", tc.typeVal, got, tc.want)
		}
	}
}

// writeSkill creates a SKILL.md file in skillsDir/dirName.
// If dirName is empty, writes directly to skillsDir.
func writeSkill(t *testing.T, parentDir, dirName, content string) {
	t.Helper()
	dir := parentDir
	if dirName != "" {
		dir = filepath.Join(parentDir, dirName)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write SKILL.md in %s: %v", dir, err)
	}
}
