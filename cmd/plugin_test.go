package cmd

import (
	"errors"
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
	type hookGroup struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"hooks"`
	}
	hooks := decodeJSON[struct {
		Hooks struct {
			SessionStart []hookGroup `json:"SessionStart"`
			PreToolUse   []hookGroup `json:"PreToolUse"`
			PostToolUse  []hookGroup `json:"PostToolUse"`
		} `json:"hooks"`
	}](t, string(readPluginFile(t, "hooks", "hooks.json")))

	if len(hooks.Hooks.SessionStart) != 1 {
		t.Fatalf("SessionStart = %+v, want one hook group", hooks.Hooks.SessionStart)
	}
	startGroup := hooks.Hooks.SessionStart[0]
	if startGroup.Matcher != "" {
		t.Errorf("SessionStart matcher = %q, want empty so every session-start source runs", startGroup.Matcher)
	}
	if len(startGroup.Hooks) != 1 || startGroup.Hooks[0].Type != "command" {
		t.Fatalf("SessionStart hooks = %+v, want one command hook", startGroup.Hooks)
	}
	startCmd := startGroup.Hooks[0].Command
	if !strings.Contains(startCmd, "${CLAUDE_PLUGIN_ROOT}") {
		t.Errorf("SessionStart command = %q, want it rooted at the plugin directory", startCmd)
	}
	startScript := strings.ReplaceAll(strings.ReplaceAll(startCmd, `"${CLAUDE_PLUGIN_ROOT}"`, repoRoot(t)), "/", string(filepath.Separator))
	if _, err := os.Stat(startScript); err != nil {
		t.Errorf("SessionStart hook script: %v", err)
	}

	if len(hooks.Hooks.PreToolUse) != 1 {
		t.Fatalf("PreToolUse = %+v, want one matcher group", hooks.Hooks.PreToolUse)
	}
	preGroup := hooks.Hooks.PreToolUse[0]
	for _, tool := range []string{"Read", "Glob", "Grep"} {
		if !strings.Contains(preGroup.Matcher, tool) {
			t.Errorf("PreToolUse matcher = %q, want it to match %s", preGroup.Matcher, tool)
		}
	}
	if len(preGroup.Hooks) != 1 || preGroup.Hooks[0].Type != "command" {
		t.Fatalf("PreToolUse hooks = %+v, want one command hook", preGroup.Hooks)
	}
	preCmd := preGroup.Hooks[0].Command
	if !strings.Contains(preCmd, "${CLAUDE_PLUGIN_ROOT}") {
		t.Errorf("PreToolUse command = %q, want it rooted at the plugin directory", preCmd)
	}
	preScript := strings.ReplaceAll(strings.ReplaceAll(preCmd, `"${CLAUDE_PLUGIN_ROOT}"`, repoRoot(t)), "/", string(filepath.Separator))
	if _, err := os.Stat(preScript); err != nil {
		t.Errorf("PreToolUse hook script: %v", err)
	}

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

	bin := buildTestDocdag(t)
	env := append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, payload := range []string{
		`{"tool_name":"Edit","tool_input":{"file_path":"/tmp/somewhere/main.go"}}`,
		`{"tool_name":"Write","tool_input":{"file_path":"/tmp/somewhere/notes.md"}}`,
		`{"tool_name":"Edit","tool_input":{}}`,
	} {
		cmd := exec.Command("sh", script)
		cmd.Stdin = strings.NewReader(payload)
		cmd.Dir = repoRoot(t)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("hook on %s: %v (output %q)", payload, err, out)
		}
		if len(out) != 0 {
			t.Errorf("hook on %s wrote %q, want silence for an unrelated path", payload, out)
		}
	}
}

// TestPluginHookLintsAConfigurationEdit covers the hook's other branch: what
// breaks when docdag.yaml changes is the rules, so the edit is linted rather
// than validated. The stub stands in for the binary, so the test is about the
// branch rather than about what lint happens to report today.
func TestPluginHookLintsAConfigurationEdit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hook reads its payload with jq")
	}
	project := t.TempDir()
	bin := filepath.Join(project, "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", bin, err)
	}
	report := "docdag.yaml:3: WARN tautological_rule everything: constrains nothing"
	stub := "#!/bin/sh\ncase \"$1\" in lint) echo \"" + report + "\"; exit 2 ;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docdag"), []byte(stub), 0o750); err != nil {
		t.Fatalf("write the stub: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "docdag.yaml"), []byte("preset: adr\n"), 0o600); err != nil {
		t.Fatalf("write the configuration: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "docdag-validate.sh"))
	cmd.Stdin = strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"` + filepath.Join(project, "docdag.yaml") + `"}}`)
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()

	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 2 {
		t.Fatalf("hook exited %v, want 2 so the report reaches the agent (output %q)", err, out)
	}
	if !strings.Contains(string(out), report) {
		t.Errorf("hook wrote %q, want the lint report", out)
	}
}

// TestPluginHookLintsTheConfigurationThatWasEdited pins the invocation rather
// than the report: a nested docdag.yaml is a different configuration from the
// project root's, and linting the wrong one reports findings about lines
// nobody touched.
func TestPluginHookLintsTheConfigurationThatWasEdited(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hook reads its payload with jq")
	}
	project := t.TempDir()
	bin := filepath.Join(project, "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", bin, err)
	}
	nested := filepath.Join(project, "standard", "docdag.yaml")
	if err := os.MkdirAll(filepath.Dir(nested), 0o750); err != nil {
		t.Fatalf("mkdir for %s: %v", nested, err)
	}
	for _, path := range []string{filepath.Join(project, "docdag.yaml"), nested} {
		if err := os.WriteFile(path, []byte("preset: adr\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	// The stub records what it was asked, so the assertion is about the
	// command line rather than about what lint reports today.
	args := filepath.Join(project, "args")
	stub := "#!/bin/sh\ncase \"$1\" in lint) printf '%s\\n' \"$*\" > " + args + "; exit 0 ;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docdag"), []byte(stub), 0o750); err != nil {
		t.Fatalf("write the stub: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "docdag-validate.sh"))
	cmd.Stdin = strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"` + nested + `"}}`)
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("hook: %v (output %q)", err, out)
	}

	recorded, err := os.ReadFile(args)
	if err != nil {
		t.Fatalf("read the recorded arguments: %v", err)
	}
	if want := "lint --config " + nested; strings.TrimSpace(string(recorded)) != want {
		t.Errorf("docdag ran %q, want %q: the edited file is the configuration to lint", strings.TrimSpace(string(recorded)), want)
	}
}

// TestPluginHookReportsAConfigurationItCannotRead covers exit code 3 from
// docdag: an edit that leaves docdag.yaml invalid exits 3, and the hook prints
// docdag's stderr and exits 2 so Claude Code blocks and sees it.
func TestPluginHookReportsAConfigurationItCannotRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hook reads its payload with jq")
	}
	project := t.TempDir()
	bin := filepath.Join(project, "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", bin, err)
	}
	stub := "#!/bin/sh\ncase \"$1\" in lint) echo \"bad configuration\" >&2; exit 3 ;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docdag"), []byte(stub), 0o750); err != nil {
		t.Fatalf("write the stub: %v", err)
	}
	config := filepath.Join(project, "docdag.yaml")
	if err := os.WriteFile(config, []byte("edges: 3\n"), 0o600); err != nil {
		t.Fatalf("write the configuration: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "docdag-validate.sh"))
	cmd.Stdin = strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"` + config + `"}}`)
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("hook err = %v, want exit code 2 (output %q)", err, out)
	}
	if !strings.Contains(string(out), "bad configuration") {
		t.Errorf("hook wrote %q, want stderr from docdag", out)
	}
}

func TestPluginHookFailsClosedOnUnexpectedValidateExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hook reads its payload with jq")
	}
	project := t.TempDir()
	bin := filepath.Join(project, "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", bin, err)
	}
	stub := "#!/bin/sh\ncase \"$1\" in validate) echo \"cannot read corpus\" >&2; exit 3 ;; esac\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "docdag"), []byte(stub), 0o750); err != nil {
		t.Fatalf("write the stub: %v", err)
	}
	docs := filepath.Join(project, "docs", "adr")
	if err := os.MkdirAll(docs, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", docs, err)
	}
	doc := filepath.Join(docs, "0001-test.md")
	if err := os.WriteFile(doc, []byte("# Test\n"), 0o600); err != nil {
		t.Fatalf("write doc: %v", err)
	}

	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "docdag-validate.sh"))
	cmd.Stdin = strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"` + doc + `"}}`)
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+project, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
		t.Fatalf("hook err = %v, want exit 2 (output %q)", err, out)
	}
	if !strings.Contains(string(out), "cannot read corpus") {
		t.Errorf("hook wrote %q, want stderr from docdag", out)
	}
}

func TestPluginHookMissingNoticeSaysEnforcementOff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	project := t.TempDir()
	cmd := exec.Command("sh", filepath.Join(repoRoot(t), "scripts", "docdag-validate.sh"))
	cmd.Stdin = strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"test.md"}}`)
	cmd.Dir = project
	cmd.Env = []string{"PATH=/nonexistent"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("hook failed: %v", err)
	}
	if !strings.Contains(string(out), "enforcement is OFF") {
		t.Errorf("output = %q, want it to say enforcement is OFF", out)
	}
}

func buildTestDocdag(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain is not on PATH")
	}
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", bin, err)
	}
	binary := filepath.Join(bin, "docdag")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./docdag")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build docdag: %v (output %q)", err, out)
	}
	return bin
}

func TestPluginSessionStartHookBehaviours(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	script := filepath.Join(repoRoot(t), "scripts", "docdag-session-start.sh")
	if info, err := os.Stat(script); err != nil {
		t.Fatalf("hook script: %v", err)
	} else if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook script mode = %v, want it executable", info.Mode().Perm())
	}

	t.Run("silent when project has no corpus", func(t *testing.T) {
		project := t.TempDir()
		cmd := exec.Command("sh", script)
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+project)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("hook failed: %v (output %q)", err, out)
		}
		if len(out) != 0 {
			t.Errorf("output = %q, want silence without a corpus", out)
		}
	})

	t.Run("missing docdag prints enforcement OFF notice to stdout", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			init func(t *testing.T, dir string)
		}{
			{
				name: "docs/adr directory",
				init: func(t *testing.T, dir string) {
					if err := os.MkdirAll(filepath.Join(dir, "docs", "adr"), 0o750); err != nil {
						t.Fatal(err)
					}
				},
			},
			{
				name: "root docdag.yaml",
				init: func(t *testing.T, dir string) {
					if err := os.WriteFile(filepath.Join(dir, "docdag.yaml"), []byte("dir: decisions\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				project := t.TempDir()
				tc.init(t, project)
				cmd := exec.Command("sh", script)
				cmd.Dir = project
				cmd.Env = []string{"CLAUDE_PROJECT_DIR=" + project, "PATH=/nonexistent"}
				var stdout, stderr strings.Builder
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("hook failed: %v (stderr=%q)", err, stderr.String())
				}
				if stderr.Len() != 0 {
					t.Errorf("stderr = %q, want empty", stderr.String())
				}
				if stdout.String() != "docdag: not on PATH, enforcement is OFF\n" {
					t.Errorf("stdout = %q, want exact notice line", stdout.String())
				}
			})
		}
	})

	bin := buildTestDocdag(t)
	envWithBin := func(project string) []string {
		return append(os.Environ(), "CLAUDE_PROJECT_DIR="+project, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	t.Run("passing corpus prints prime brief starting with validate OK", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docs/adr/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
		})
		cmd := exec.Command("sh", script)
		cmd.Dir = project
		cmd.Env = envWithBin(project)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("hook failed: %v", err)
		}
		got := lines(string(out))
		if len(got) < 3 || got[0] != "docdag validate: OK: 1 docs, 0 typed edges, no cycles" {
			t.Fatalf("stdout lines = %q, want prime brief starting with OK summary", got)
		}
		for _, want := range []string{"docdag context", "docdag query --binding --fields id,title,status"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("stdout = %q, want %q", out, want)
			}
		}
	})

	t.Run("failing corpus prints FAILED, follow-up, and prime brief and exits 0", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docs/adr/0001-first.md":  "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
			"docs/adr/0002-second.md": "---\ntitle: Second\nstatus: accepted\nsupersedes:\n  - 0001\ndate: 2025-01-02\n---\n\n# Second\n",
		})
		cmd := exec.Command("sh", script)
		cmd.Dir = project
		cmd.Env = envWithBin(project)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("hook failed: %v", err)
		}
		got := lines(string(out))
		if len(got) < 3 || got[0] != "docdag validate: FAILED: 1 error" || got[1] != "run docdag validate to see the findings" {
			t.Fatalf("stdout lines = %q, want FAILED header + follow-up + prime brief", got)
		}
		if !strings.Contains(string(out), "docdag context") {
			t.Errorf("stdout = %q, want docdag context guidance", out)
		}
	})

	t.Run("older docdag without prime falls back to validate summary", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docs/adr/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
		})
		oldBin := filepath.Join(t.TempDir(), "bin")
		if err := os.MkdirAll(oldBin, 0o750); err != nil {
			t.Fatal(err)
		}
		stub := "#!/bin/sh\ncase \"$1\" in\n  prime) echo 'Error: unknown command \"prime\" for \"docdag\"' >&2; exit 2 ;;\n  validate) echo 'OK: 1 docs, 0 typed edges, no cycles'; exit 0 ;;\nesac\nexit 0\n"
		if err := os.WriteFile(filepath.Join(oldBin, "docdag"), []byte(stub), 0o750); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", script)
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+project, "PATH="+oldBin+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("hook failed: %v", err)
		}
		assertLines(t, "fallback validate", lines(string(out)), []string{
			"docdag validate: OK: 1 docs, 0 typed edges, no cycles",
		})
	})

	t.Run("unexpected exit prints exited N and first line of stderr and exits 0", func(t *testing.T) {
		project := t.TempDir()
		if err := os.MkdirAll(filepath.Join(project, "docs", "adr"), 0o750); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", script)
		cmd.Dir = project
		cmd.Env = envWithBin(project)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("hook failed: %v", err)
		}
		got := lines(string(out))
		if len(got) != 1 || !strings.HasPrefix(got[0], "docdag validate: exited 3: ") {
			t.Errorf("stdout lines = %q, want one 'docdag validate: exited 3: ...' line", got)
		}
	})
}

func TestPluginPostToolUseHookBehaviours(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hook reads its payload with jq")
	}
	script := filepath.Join(repoRoot(t), "scripts", "docdag-validate.sh")
	bin := buildTestDocdag(t)

	runHook := func(t *testing.T, project, pathEnv, filePath string) (int, string, string) {
		t.Helper()
		cmd := exec.Command("sh", script)
		cmd.Stdin = strings.NewReader(`{"tool_name":"Edit","tool_input":{"file_path":"` + filePath + `"}}`)
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "CLAUDE_PROJECT_DIR="+project, "PATH="+pathEnv)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		code := 0
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("run hook: %v", err)
			}
			code = exitErr.ExitCode()
		}
		return code, stdout.String(), stderr.String()
	}

	pathWithBin := bin + string(os.PathListSeparator) + os.Getenv("PATH")

	t.Run("spec preset validates edited clause under per-kind directory", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docdag.yaml":              "preset: spec\n",
			"spec/clauses/UZ-V-001.md": "---\ntitle: Broken clause\nkind: clause\nmodality: MUST\nstatus: accepted\ndate: 2026-01-05\n---\n\n# Broken clause\n",
		})
		clause := filepath.Join(project, "spec", "clauses", "UZ-V-001.md")
		code, stdout, stderr := runHook(t, project, pathWithBin, clause)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "ERROR orphan_must") {
			t.Errorf("stderr = %q, want validation finding for broken clause", stderr)
		}
	})

	t.Run("user-declared per-kind dir validates edited document", func(t *testing.T) {
		cfg := `kinds:
  clause:
    dir: custom/clauses
    id: '^UZ-[A-Z]-\d{3}$'
    status_values: [proposed, accepted, superseded]
edges:
  - name: supersedes
    key: supersedes
    acyclic: true
    direction: forward
    from: [clause]
    to: [clause]
`
		project := writeDocs(t, map[string]string{
			"docdag.yaml":                cfg,
			"custom/clauses/UZ-V-001.md": "---\ntitle: Broken clause\nkind: clause\nstatus: accepted\nsupersedes:\n  - UZ-V-999\n---\n\n# Broken clause\n",
		})
		clause := filepath.Join(project, "custom", "clauses", "UZ-V-001.md")
		code, stdout, stderr := runHook(t, project, pathWithBin, clause)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "ERROR dangling_ref") {
			t.Errorf("stderr = %q, want validation finding for broken clause", stderr)
		}
	})

	t.Run("top-level dir validates edited document", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docdag.yaml":             "dir: decisions\n",
			"decisions/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
			"decisions/0002-bad.md":   "---\ntitle: Bad\nstatus: accepted\nsupersedes:\n  - 0099\ndate: 2025-01-02\n---\n\n# Bad\n",
		})
		doc := filepath.Join(project, "decisions", "0002-bad.md")
		code, stdout, stderr := runHook(t, project, pathWithBin, doc)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "ERROR dangling_ref") {
			t.Errorf("stderr = %q, want validation finding", stderr)
		}
	})

	t.Run("well-known adr directory validates without docdag.yaml", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docs/decisions/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
			"docs/decisions/0002-bad.md":   "---\ntitle: Bad\nstatus: accepted\nsupersedes:\n  - 0099\ndate: 2025-01-02\n---\n\n# Bad\n",
		})
		doc := filepath.Join(project, "docs", "decisions", "0002-bad.md")
		code, stdout, stderr := runHook(t, project, pathWithBin, doc)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "ERROR dangling_ref") {
			t.Errorf("stderr = %q, want validation finding", stderr)
		}
	})

	t.Run("unrelated markdown outside document directories exits 0 silently", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			files map[string]string
		}{
			{
				name: "spec corpus with failing clause",
				files: map[string]string{
					"docdag.yaml":              "preset: spec\n",
					"spec/clauses/UZ-V-001.md": "---\ntitle: Broken clause\nkind: clause\nmodality: MUST\nstatus: accepted\ndate: 2026-01-05\n---\n\n# Broken clause\n",
					"README.md":                "# Readme\n",
				},
			},
			{
				name: "adr corpus with failing decision",
				files: map[string]string{
					"docs/adr/0001-bad.md": "---\ntitle: Bad\nstatus: accepted\nsupersedes:\n  - 0099\ndate: 2025-01-02\n---\n\n# Bad\n",
					"README.md":            "# Readme\n",
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				project := writeDocs(t, tc.files)
				readme := filepath.Join(project, "README.md")
				code, stdout, stderr := runHook(t, project, pathWithBin, readme)
				if code != 0 || stdout != "" || stderr != "" {
					t.Errorf("got code=%d stdout=%q stderr=%q, want exit 0 with silence", code, stdout, stderr)
				}
			})
		}
	})

	t.Run("docdag.yaml edit runs lint instead of validate", func(t *testing.T) {
		cfg := `rules:
  - name: tautological_rule
    severity: warn
    when:
      attr:
        status:
          eq: accepted
    message: constrains nothing
`
		project := writeDocs(t, map[string]string{
			"docdag.yaml": cfg,
		})
		cfgPath := filepath.Join(project, "docdag.yaml")
		code, stdout, stderr := runHook(t, project, pathWithBin, cfgPath)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout = %q, want empty", stdout)
		}
		if !strings.Contains(stderr, "docdag.yaml") {
			t.Errorf("stderr = %q, want lint report", stderr)
		}
	})

	t.Run("clean clause exits 0 silently when another clause in spec corpus fails", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docdag.yaml":              "preset: spec\n",
			"spec/topics/evidence.md":  "---\ntitle: Evidence\nkind: topic\nid: topic/evidence\ndate: 2026-01-03\n---\n\n# Evidence\n",
			"spec/pm/pm-0001.md":       "---\ntitle: Incident\nkind: pm\nstatus: published\ndate: 2026-02-14\n---\n\n# Incident\n",
			"spec/conform/uz-v-001.md": "---\ntitle: Test 001\nkind: conform\nid: conform/uz-v-001\nenforces:\n  - UZ-V-001\ndate: 2026-02-01\ntest: test.sh\n---\n\n# Test 001\n",
			"spec/clauses/UZ-V-001.md": "---\ntitle: Clean clause\nkind: clause\nmodality: MUST\nstatus: accepted\nabout:\n  - topic/evidence\ncounterexample:\n  - pm-0001\ndate: 2026-01-05\n---\n\n# Clean clause\n",
			"spec/clauses/UZ-V-002.md": "---\ntitle: Broken clause\nkind: clause\nmodality: MUST\nstatus: accepted\nabout:\n  - topic/evidence\ncounterexample:\n  - pm-0001\ndate: 2026-01-05\n---\n\n# Broken clause\n",
		})
		clean := filepath.Join(project, "spec", "clauses", "UZ-V-001.md")
		code, stdout, stderr := runHook(t, project, pathWithBin, clean)
		if code != 0 || stdout != "" || stderr != "" {
			t.Fatalf("got code=%d stdout=%q stderr=%q, want silent exit 0 when only another document fails", code, stdout, stderr)
		}
	})

	t.Run("older docdag without --if-managed does not block markdown edits", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"README.md": "# Readme\n",
		})
		oldBin := filepath.Join(t.TempDir(), "bin")
		if err := os.MkdirAll(oldBin, 0o750); err != nil {
			t.Fatal(err)
		}
		stub := "#!/bin/sh\necho 'Error: unknown flag: --if-managed' >&2\nexit 2\n"
		if err := os.WriteFile(filepath.Join(oldBin, "docdag"), []byte(stub), 0o750); err != nil {
			t.Fatal(err)
		}
		readme := filepath.Join(project, "README.md")
		code, stdout, stderr := runHook(t, project, oldBin+string(os.PathListSeparator)+os.Getenv("PATH"), readme)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stdout=%q stderr=%q)", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "enforcement is OFF") {
			t.Errorf("stderr = %q, want non-blocking enforcement OFF notice", stderr)
		}
	})

	t.Run("paths with spaces validate cleanly", func(t *testing.T) {
		project := writeDocs(t, map[string]string{
			"docdag.yaml":                          "dir: my decisions\n",
			"my decisions/0001-first-decision.md":  "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
			"my decisions/0002-broken-decision.md": "---\ntitle: Broken\nstatus: accepted\nsupersedes:\n  - 0099\ndate: 2025-01-02\n---\n\n# Broken\n",
		})
		doc := filepath.Join(project, "my decisions", "0002-broken-decision.md")
		code, stdout, stderr := runHook(t, project, pathWithBin, doc)
		if code != 2 {
			t.Fatalf("exit code = %d, want 2 (stdout=%q stderr=%q)", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "ERROR dangling_ref") {
			t.Errorf("stderr = %q, want dangling_ref for path with spaces", stderr)
		}
	})
}

func TestPluginPreReadHookAdvisesWithoutBlocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hook reads its payload with jq")
	}
	script := filepath.Join(repoRoot(t), "scripts", "docdag-pre-read.sh")
	if info, err := os.Stat(script); err != nil {
		t.Fatalf("hook script: %v", err)
	} else if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("hook script mode = %v, want it executable", info.Mode().Perm())
	}

	bin := buildTestDocdag(t)
	project := writeDocs(t, map[string]string{
		"docs/adr/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
		"docs/adr/README.md":     "# ADRs\n",
		"README.md":              "# Root\n",
	})
	env := append(os.Environ(), "CLAUDE_PROJECT_DIR="+project, "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	runPre := func(t *testing.T, payload string) string {
		t.Helper()
		cmd := exec.Command("sh", script)
		cmd.Stdin = strings.NewReader(payload)
		cmd.Dir = project
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("hook failed on %s: %v (output=%q)", payload, err, out)
		}
		return string(out)
	}

	t.Run("managed Read emits non-blocking additionalContext without permissionDecision", func(t *testing.T) {
		doc := filepath.Join(project, "docs", "adr", "0001-first.md")
		out := runPre(t, `{"tool_name":"Read","tool_input":{"file_path":"`+doc+`"}}`)
		if strings.Contains(out, "permissionDecision") {
			t.Errorf("output = %q, must omit permissionDecision", out)
		}
		resp := decodeJSON[struct {
			HookSpecificOutput struct {
				HookEventName     string `json:"hookEventName"`
				AdditionalContext string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}](t, out)
		if resp.HookSpecificOutput.HookEventName != "PreToolUse" {
			t.Errorf("hookEventName = %q, want PreToolUse", resp.HookSpecificOutput.HookEventName)
		}
		if !strings.Contains(resp.HookSpecificOutput.AdditionalContext, "docdag context") {
			t.Errorf("additionalContext = %q, want docdag context tip", resp.HookSpecificOutput.AdditionalContext)
		}
	})

	t.Run("managed Glob emits additionalContext", func(t *testing.T) {
		out := runPre(t, `{"tool_name":"Glob","tool_input":{"path":"docs/adr","pattern":"*.md"}}`)
		if !strings.Contains(out, "docdag query --binding") {
			t.Errorf("output = %q, want docdag query --binding tip", out)
		}
	})

	t.Run("unrelated or exempt paths stay silent", func(t *testing.T) {
		for _, payload := range []string{
			`{"tool_name":"Read","tool_input":{"file_path":"` + filepath.Join(project, "README.md") + `"}}`,
			`{"tool_name":"Read","tool_input":{"file_path":"` + filepath.Join(project, "docs", "adr", "README.md") + `"}}`,
			`{"tool_name":"Grep","tool_input":{"pattern":"foo"}}`,
		} {
			if out := runPre(t, payload); out != "" {
				t.Errorf("payload %s produced %q, want silence", payload, out)
			}
		}
	})
}

func TestAntigravityPluginManifestHooksAndRules(t *testing.T) {
	claudeManifest := decodeJSON[struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}](t, string(readPluginFile(t, ".claude-plugin", "plugin.json")))

	agyManifest := decodeJSON[struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
	}](t, string(readPluginFile(t, ".antigravity-plugin", "plugin.json")))

	if agyManifest.Name != claudeManifest.Name || agyManifest.Version != claudeManifest.Version {
		t.Errorf("agy manifest = %+v, want name=%q version=%q matching Claude plugin", agyManifest, claudeManifest.Name, claudeManifest.Version)
	}
	if agyManifest.Description == "" {
		t.Error("agy manifest description is empty")
	}

	rules := string(readPluginFile(t, "rules", "AGENTS.md"))
	for _, want := range []string{"docdag prime", "docdag context", "docdag query --binding", "docdag validate --touching"} {
		if !strings.Contains(rules, want) {
			t.Errorf("rules/AGENTS.md does not mention %q", want)
		}
	}

	agyHooks := decodeJSON[map[string]struct {
		SessionStart []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"SessionStart"`
		PostToolUse []struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"PostToolUse"`
		PostInvocation []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"PostInvocation"`
	}](t, string(readPluginFile(t, "hooks.json")))

	hook, ok := agyHooks["docdag"]
	if !ok {
		t.Fatalf("hooks.json missing top-level \"docdag\" entry: %+v", agyHooks)
	}
	if len(hook.SessionStart) != 1 || len(hook.PostToolUse) != 2 || len(hook.PostInvocation) != 1 {
		t.Fatalf("unexpected agy hook counts: %+v", hook)
	}
	for _, rel := range []string{
		hook.SessionStart[0].Command,
		hook.PostToolUse[0].Hooks[0].Command,
		hook.PostToolUse[1].Hooks[0].Command,
		hook.PostInvocation[0].Command,
	} {
		path := filepath.Join(repoRoot(t), filepath.FromSlash(strings.TrimPrefix(rel, "./")))
		info, err := os.Stat(path)
		if err != nil {
			t.Errorf("hook command %q: %v", rel, err)
			continue
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
			t.Errorf("hook script %s is not executable", path)
		}
	}
}

func TestAntigravityHooksRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the hook is a POSIX shell script")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the hooks read payloads with jq")
	}

	bin := buildTestDocdag(t)
	startScript := filepath.Join(repoRoot(t), "scripts", "agy-session-start.sh")
	postToolScript := filepath.Join(repoRoot(t), "scripts", "agy-post-tool.sh")
	postInvScript := filepath.Join(repoRoot(t), "scripts", "agy-post-invocation.sh")
	stateDir := t.TempDir()

	project := writeDocs(t, map[string]string{
		"docs/adr/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
		"docs/adr/0002-bad.md":   "---\ntitle: Bad\nstatus: accepted\nsupersedes:\n  - 0099\ndate: 2025-01-02\n---\n\n# Bad\n",
	})
	env := append(os.Environ(),
		"PLUGIN_DATA="+stateDir,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	runAgy := func(t *testing.T, script, payload string) string {
		t.Helper()
		cmd := exec.Command("sh", script)
		cmd.Stdin = strings.NewReader(payload)
		cmd.Dir = repoRoot(t)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v (output=%q)", filepath.Base(script), err, out)
		}
		return string(out)
	}

	type injectResp struct {
		InjectSteps []struct {
			SystemMessage struct {
				SystemMessage string `json:"systemMessage"`
			} `json:"systemMessage"`
		} `json:"injectSteps"`
	}

	t.Run("SessionStart injects prime brief as systemMessage", func(t *testing.T) {
		out := runAgy(t, startScript, `{"cwd":"`+project+`","workspacePaths":["`+project+`"]}`)
		resp := decodeJSON[injectResp](t, out)
		if len(resp.InjectSteps) != 1 {
			t.Fatalf("injectSteps = %+v, want 1", resp.InjectSteps)
		}
		msg := resp.InjectSteps[0].SystemMessage.SystemMessage
		if !strings.Contains(msg, "docdag validate: FAILED:") || !strings.Contains(msg, "docdag context") {
			t.Errorf("systemMessage = %q, want prime brief", msg)
		}
	})

	t.Run("PostToolUse queues validation findings and PostInvocation drains once", func(t *testing.T) {
		bad := filepath.Join(project, "docs", "adr", "0002-bad.md")
		toolOut := strings.TrimSpace(runAgy(t, postToolScript,
			`{"conversationId":"conv-edit-1","cwd":"`+project+`","toolCall":{"name":"edit_file","args":{"Path":"`+bad+`"}}}`))
		if toolOut != "{}" {
			t.Errorf("PostToolUse stdout = %q, want {}", toolOut)
		}
		invOut := runAgy(t, postInvScript, `{"conversationId":"conv-edit-1"}`)
		resp := decodeJSON[injectResp](t, invOut)
		if len(resp.InjectSteps) != 1 || !strings.Contains(resp.InjectSteps[0].SystemMessage.SystemMessage, "ERROR dangling_ref") {
			t.Fatalf("PostInvocation = %q, want dangling_ref systemMessage", invOut)
		}
		if again := strings.TrimSpace(runAgy(t, postInvScript, `{"conversationId":"conv-edit-1"}`)); again != "{}" {
			t.Errorf("second PostInvocation = %q, want {}", again)
		}
	})

	t.Run("PostToolUse read hint fires once per conversation", func(t *testing.T) {
		first := filepath.Join(project, "docs", "adr", "0001-first.md")
		payload := `{"conversationId":"conv-read-1","cwd":"` + project + `","toolCall":{"name":"read_file","args":{"Path":"` + first + `"}}}`
		if out := strings.TrimSpace(runAgy(t, postToolScript, payload)); out != "{}" {
			t.Errorf("PostToolUse stdout = %q, want {}", out)
		}
		invOut := runAgy(t, postInvScript, `{"conversationId":"conv-read-1"}`)
		resp := decodeJSON[injectResp](t, invOut)
		if len(resp.InjectSteps) != 1 || !strings.Contains(resp.InjectSteps[0].SystemMessage.SystemMessage, "docdag context") {
			t.Fatalf("first read PostInvocation = %q, want docdag context hint", invOut)
		}
		// Second read in the same conversation must be deduplicated.
		_ = runAgy(t, postToolScript, payload)
		if again := strings.TrimSpace(runAgy(t, postInvScript, `{"conversationId":"conv-read-1"}`)); again != "{}" {
			t.Errorf("second read PostInvocation = %q, want {} (deduplicated)", again)
		}
	})
}
