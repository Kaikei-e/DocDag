package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot is where the shipped Claude Code plugin lives, one level above the
// package under test.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repository root: %v", err)
	}
	return root
}

func readPluginFile(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t)}, parts...)...)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return content
}

func TestPluginManifestNamesThePlugin(t *testing.T) {
	manifest := decodeJSON[struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
	}](t, string(readPluginFile(t, ".claude-plugin", "plugin.json")))

	if manifest.Name != "docdag" {
		t.Errorf("name = %q, want docdag: the name is the skill namespace", manifest.Name)
	}
	if manifest.Description == "" || manifest.Version == "" {
		t.Errorf("manifest = %+v, want a description and a version", manifest)
	}
}

func TestPluginMarketplaceListsThePluginAtTheRepositoryRoot(t *testing.T) {
	marketplace := decodeJSON[struct {
		Name  string `json:"name"`
		Owner struct {
			Name string `json:"name"`
		} `json:"owner"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}](t, string(readPluginFile(t, ".claude-plugin", "marketplace.json")))

	if marketplace.Name == "" || marketplace.Owner.Name == "" {
		t.Errorf("marketplace = %+v, want a name and an owner", marketplace)
	}
	if len(marketplace.Plugins) != 1 {
		t.Fatalf("plugins = %+v, want the one this repository ships", marketplace.Plugins)
	}
	if marketplace.Plugins[0].Name != "docdag" || marketplace.Plugins[0].Source != "./" {
		t.Errorf("plugin = %+v, want docdag at the repository root", marketplace.Plugins[0])
	}
}

func TestPluginHookRunsTheScriptAfterAnEditOrAWrite(t *testing.T) {
	hooks := decodeJSON[struct {
		Hooks struct {
			PostToolUse []struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"PostToolUse"`
		} `json:"hooks"`
	}](t, string(readPluginFile(t, "hooks", "hooks.json")))

	if len(hooks.Hooks.PostToolUse) != 1 {
		t.Fatalf("PostToolUse = %+v, want one matcher group", hooks.Hooks.PostToolUse)
	}
	group := hooks.Hooks.PostToolUse[0]
	for _, tool := range []string{"Edit", "Write"} {
		if !strings.Contains(group.Matcher, tool) {
			t.Errorf("matcher = %q, want it to match %s", group.Matcher, tool)
		}
	}
	if len(group.Hooks) != 1 || group.Hooks[0].Type != "command" {
		t.Fatalf("hooks = %+v, want one command hook", group.Hooks)
	}
	command := group.Hooks[0].Command
	if !strings.Contains(command, "${CLAUDE_PLUGIN_ROOT}") {
		t.Errorf("command = %q, want it rooted at the plugin directory", command)
	}
	script := strings.ReplaceAll(strings.ReplaceAll(command, `"${CLAUDE_PLUGIN_ROOT}"`, repoRoot(t)), "/", string(filepath.Separator))
	if _, err := os.Stat(script); err != nil {
		t.Errorf("hook script: %v", err)
	}
}

func TestPluginSkillTeachesTheVocabulary(t *testing.T) {
	skill := string(readPluginFile(t, "skills", "docdag", "SKILL.md"))

	if !strings.HasPrefix(skill, "---\n") || !strings.Contains(skill, "\ndescription:") {
		t.Fatalf("SKILL.md = %q, want frontmatter carrying a description", skill[:min(len(skill), 120)])
	}
	for _, want := range []string{"docdag context", "docdag validate", "docdag resolve", "docdag query --binding", "Bash(docdag *)"} {
		if !strings.Contains(skill, want) {
			t.Errorf("SKILL.md does not teach %q", want)
		}
	}
}

func TestPluginHookIgnoresPathsOutsideTheDocumentsDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hook reads its payload with jq")
	}
	script := filepath.Join(repoRoot(t), "scripts", "docdag-validate.sh")
	if info, err := os.Stat(script); err != nil {
		t.Fatalf("hook script: %v", err)
	} else if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook script mode = %v, want it executable", info.Mode().Perm())
	}

	for _, payload := range []string{
		`{"tool_name":"Edit","tool_input":{"file_path":"/tmp/somewhere/main.go"}}`,
		`{"tool_name":"Write","tool_input":{"file_path":"/tmp/somewhere/notes.md"}}`,
		`{"tool_name":"Edit","tool_input":{}}`,
	} {
		cmd := exec.Command("sh", script)
		cmd.Stdin = strings.NewReader(payload)
		cmd.Dir = repoRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("hook on %s: %v (output %q)", payload, err, out)
		}
		if len(out) != 0 {
			t.Errorf("hook on %s wrote %q, want silence for an unrelated path", payload, out)
		}
	}
}
