package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/model"
)

const (
	flagIfPresent = "if-present"
	flagHint      = "hint"
)

func newPrimeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prime",
		Short: "Print a compact, corpus-adaptive brief for coding agents",
		Long: "Print a compact brief describing the corpus health, layout, schema, and tailored\n" +
			"docdag commands so coding agents query the graph directly instead of scanning\n" +
			"directories. With --if-present, exit 0 silently when no corpus is discovered.\n" +
			"With --hint <path>, inspect configuration only (without parsing documents) and\n" +
			"print a short command tip when <path> or a glob touches a managed directory.",
		Example: "  docdag prime\n" +
			"  docdag prime --if-present\n" +
			"  docdag prime --format json\n" +
			"  docdag prime --if-present --hint docs/adr/0001.md",
		Args: cobra.NoArgs,
		RunE: runPrime,
	}
	cmd.Flags().Bool(flagIfPresent, false, "exit 0 silently when no document corpus is discovered")
	cmd.Flags().String(flagHint, "", "print a short docdag command tip if this path or glob touches a managed directory, without parsing documents")
	addAsOfFlag(cmd, "the day HEAD was committed on")
	addAtFlag(cmd)
	return cmd
}

func runPrime(cmd *cobra.Command, _ []string) error {
	format, err := outputFormat(cmd, formatText, formatJSON)
	if err != nil {
		return err
	}
	ifPresent, err := cmd.Flags().GetBool(flagIfPresent)
	if err != nil {
		return usageErr("%v", err)
	}
	hint, err := cmd.Flags().GetString(flagHint)
	if err != nil {
		return usageErr("%v", err)
	}
	if cmd.Flags().Changed(flagHint) {
		return runPrimeHint(cmd, format, ifPresent, hint)
	}

	asOf, err := asOfCommitted(cmd)
	if err != nil {
		return err
	}
	if ifPresent {
		if _, found, err := resolvePrimeConfig(cmd, true); err != nil {
			return err
		} else if !found {
			return nil
		}
	}

	c, err := loadCorpus(cmd)
	if err != nil {
		return err
	}
	defer c.close()

	root, err := os.Getwd()
	if err != nil {
		return ioErr(fmt.Errorf("working directory: %w", err))
	}
	report := buildPrimeReport(c, root, asOf)
	out := cmd.OutOrStdout()
	if format == formatJSON {
		if err := render.PrimeJSON(out, report); err != nil {
			return ioErr(err)
		}
		return nil
	}
	if err := render.PrimeText(out, report); err != nil {
		return ioErr(err)
	}
	return nil
}

func resolvePrimeConfig(cmd *cobra.Command, ifPresent bool) (config.Config, bool, error) {
	cfg, err := resolveEffectiveConfig(cmd)
	if err != nil {
		if ifPresent && errors.Is(err, model.ErrNoDocuments) {
			return config.Config{}, false, nil
		}
		var ce *cliError
		if errors.As(err, &ce) {
			return config.Config{}, false, err
		}
		return config.Config{}, false, ioErr(err)
	}
	return cfg, true, nil
}

func runPrimeHint(cmd *cobra.Command, format string, ifPresent bool, hint string) error {
	cfg, found, err := resolvePrimeConfig(cmd, ifPresent)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return ioErr(fmt.Errorf("working directory: %w", err))
	}
	cfgPath, err := cmd.Flags().GetString(flagConfig)
	if err != nil {
		return usageErr("%v", err)
	}
	var normConfigDir string
	if cfgPath != "" {
		if absCfg, err := filepath.Abs(cfgPath); err == nil {
			normConfigDir = normalizeDir(filepath.Dir(absCfg))
		}
	}
	ph, ok := matchHint(hint, cfg, root, normConfigDir)
	if !ok {
		return nil
	}
	out := cmd.OutOrStdout()
	if format == formatJSON {
		if err := render.PrimeHintJSON(out, ph); err != nil {
			return ioErr(err)
		}
		return nil
	}
	if err := render.PrimeHintText(out, ph); err != nil {
		return ioErr(err)
	}
	return nil
}

func matchHint(hint string, cfg config.Config, root, normConfigDir string) (render.PrimeHint, bool) {
	if strings.TrimSpace(hint) == "" {
		return render.PrimeHint{}, false
	}
	dirs := corpusDirs(cfg)
	if len(dirs) == 0 {
		return render.PrimeHint{}, false
	}
	normDirs := make([]string, 0, len(dirs))
	for _, d := range dirs {
		normDirs = append(normDirs, normalizeDir(d))
	}
	normRoot := normalizeDir(root)

	if strings.ContainsAny(hint, "*?[") {
		staticDir, ok := globStaticDir(hint)
		if !ok {
			return render.PrimeHint{}, false
		}
		absDir, err := filepath.Abs(staticDir)
		if err != nil {
			return render.PrimeHint{}, false
		}
		dirNorm := normalizeDir(absDir)
		if !matchesManagedDir(dirNorm, normDirs, normRoot, normConfigDir) {
			return render.PrimeHint{}, false
		}
		display := localizedHintPath(root, normRoot, absDir, dirNorm)
		return buildPrimeHint(cfg, "dir", display), true
	}

	abs, err := filepath.Abs(hint)
	if err != nil {
		return render.PrimeHint{}, false
	}
	isDirHint := strings.HasSuffix(hint, "/") || strings.HasSuffix(hint, "\\")
	if info, err := os.Stat(abs); err == nil {
		if !info.IsDir() {
			return matchHintFile(abs, cfg, root, normRoot, normDirs)
		}
	} else if !isDirHint && filepath.Ext(abs) == ".md" {
		return matchHintFile(abs, cfg, root, normRoot, normDirs)
	}

	dirNorm := normalizeDir(abs)
	if !matchesManagedDir(dirNorm, normDirs, normRoot, normConfigDir) {
		return render.PrimeHint{}, false
	}
	display := localizedHintPath(root, normRoot, abs, dirNorm)
	return buildPrimeHint(cfg, "dir", display), true
}

func matchHintFile(abs string, cfg config.Config, root, normRoot string, normDirs []string) (render.PrimeHint, bool) {
	if filepath.Ext(abs) != ".md" {
		return render.PrimeHint{}, false
	}
	if isExemptHintFile(filepath.Base(abs), cfg) {
		return render.PrimeHint{}, false
	}
	if !slices.Contains(normDirs, normalizeDir(filepath.Dir(abs))) {
		return render.PrimeHint{}, false
	}
	display := localizedHintPath(root, normRoot, abs, normalizeDir(abs))
	return buildPrimeHint(cfg, "file", display), true
}

func isExemptHintFile(name string, cfg config.Config) bool {
	if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return true
	}
	if strings.EqualFold(name, "README.md") || strings.EqualFold(name, "index.md") || strings.EqualFold(name, "template.md") {
		return true
	}
	if cfg.Template != "" {
		tplBase := filepath.Base(cfg.Template)
		if tplBase != "." && tplBase != string(filepath.Separator) && strings.EqualFold(name, tplBase) {
			return true
		}
	}
	return false
}

// globStaticDir returns the non-root static directory prefix before the first
// glob metacharacter, avoiding false positives on repo-wide patterns like *.md.
func globStaticDir(pattern string) (string, bool) {
	idx := strings.IndexAny(pattern, "*?[")
	if idx < 0 {
		return "", false
	}
	before := pattern[:idx]
	if before == "" {
		return "", false
	}
	var dir string
	if strings.HasSuffix(before, "/") || strings.HasSuffix(before, string(filepath.Separator)) {
		dir = filepath.Clean(before)
	} else {
		dir = filepath.Dir(before)
	}
	if dir == "" || dir == "." {
		return "", false
	}
	return dir, true
}

func matchesManagedDir(dirNorm string, normDirs []string, normRoot, normConfigDir string) bool {
	if slices.Contains(normDirs, dirNorm) {
		return true
	}
	// Ancestor directories only match when the caller narrowed below the root,
	// so repo-wide searches from "." never trigger on every non-doc query.
	isNarrowed := isProperSubdir(dirNorm, normRoot) || (normConfigDir != "" && isProperSubdir(dirNorm, normConfigDir))
	if !isNarrowed {
		return false
	}
	prefix := dirPrefix(dirNorm)
	for _, d := range normDirs {
		if strings.HasPrefix(d, prefix) {
			return true
		}
	}
	return false
}

func isProperSubdir(child, parent string) bool {
	if child == "" || parent == "" || child == parent {
		return false
	}
	return strings.HasPrefix(child, dirPrefix(parent))
}

func dirPrefix(dir string) string {
	if strings.HasSuffix(dir, string(filepath.Separator)) {
		return dir
	}
	return dir + string(filepath.Separator)
}

func localizedHintPath(root, normRoot, abs, normAbs string) string {
	display := parse.LocalPath(root, abs)
	if filepath.IsAbs(display) && normAbs != "" && normRoot != "" {
		if rel := parse.LocalPath(normRoot, normAbs); !filepath.IsAbs(rel) {
			return rel
		}
	}
	return display
}

func buildPrimeHint(cfg config.Config, kind, displayPath string) render.PrimeHint {
	_, hasSupersedes := cfg.Edge(config.EdgeSupersedes)
	hasModality := slices.Contains(cfg.DeclaredFields(), config.FieldModality)
	fields := activeDeclaredFields(cfg)
	whereField := exampleWhereField(fields)

	bindingFields := "id,title,status"
	if hasModality {
		bindingFields = "id,modality,title,status"
	}

	var hint string
	if kind == "file" {
		contextRef := hintContextRef(cfg, displayPath)
		prefix := "Managed DocDag document"
		if contextRef != displayPath {
			prefix = fmt.Sprintf("Managed DocDag document (%s)", displayPath)
		}
		if hasSupersedes {
			hint = fmt.Sprintf("%s: run 'docdag context %s' for its supersession status, typed-edge neighbors, and excerpt in one call (or 'docdag query --binding --fields %s' to list active records).",
				prefix, contextRef, bindingFields)
		} else {
			hint = fmt.Sprintf("%s: run 'docdag context %s' for its typed-edge neighbors and excerpt in one call (or 'docdag query %s --fields id,title,status' to list related records).",
				prefix, contextRef, contextRef)
		}
		if whereField != "" {
			hint += fmt.Sprintf("\nFilter by declared fields with --where %s=contains:<val>.", whereField)
		}
	} else {
		if hasSupersedes {
			hint = fmt.Sprintf("Managed DocDag directory: prefer 'docdag query --binding --fields %s' (active records) or 'docdag context <ref>' (summary + neighbors) over scanning files directly.",
				bindingFields)
		} else {
			hint = "Managed DocDag directory: prefer 'docdag context <ref>' (summary + neighbors) or 'docdag query <ref> --fields id,title,status' over scanning files directly."
		}
		if whereField != "" {
			hint += fmt.Sprintf(" Filter by declared fields with --where %s=contains:<val>.", whereField)
		}
	}

	return render.PrimeHint{
		Managed: true,
		Kind:    kind,
		Path:    displayPath,
		Hint:    hint,
	}
}

func hintContextRef(cfg config.Config, displayPath string) string {
	norm := cfg.Normalizer()
	if _, ok := norm.Normalize(displayPath); ok {
		return displayPath
	}
	stem := strings.TrimSuffix(filepath.Base(displayPath), ".md")
	if id, ok := norm.Normalize(stem); ok {
		return id.String()
	}
	return displayPath
}

func buildPrimeReport(c *corpus, root string, asOf time.Time) render.PrimeReport {
	g, cfg := c.graph, c.cfg
	findings := graph.Validate(g, cfg, asOf)
	summary := graph.Summarize(g, findings)

	_, hasSupersedes := cfg.Edge(config.EdgeSupersedes)
	var bindingCount *int
	var sampleRef string
	if hasSupersedes {
		bindingIDs := graph.BindingSet(g, cfg, asOf)
		n := len(bindingIDs)
		bindingCount = &n
		if n > 0 {
			sampleRef = bindingIDs[0].String()
		}
	}
	if sampleRef == "" {
		if ids := g.NodeIDs(); len(ids) > 0 {
			sampleRef = ids[0].String()
		}
	}

	rawDirs := corpusDirs(cfg)
	dirs := make([]string, 0, len(rawDirs))
	for _, d := range rawDirs {
		dirs = append(dirs, parse.LocalPath(root, d))
	}

	var kinds []render.PrimeKind
	if cfg.Multikind() {
		names := cfg.KindNames()
		kinds = make([]render.PrimeKind, 0, len(names))
		for _, name := range names {
			spec := cfg.Kinds[name]
			kinds = append(kinds, render.PrimeKind{
				Name:      name,
				Dir:       parse.LocalPath(root, spec.Dir),
				IDPattern: spec.ID,
			})
		}
	}

	edgeTypes := cfg.EdgeTypes()
	edges := make([]string, 0, len(edgeTypes))
	for _, e := range edgeTypes {
		edges = append(edges, e.String())
	}
	slices.Sort(edges)

	fields := activeDeclaredFields(cfg)
	requiredSections := primeRequiredSections(cfg)
	hasModality := slices.Contains(cfg.DeclaredFields(), config.FieldModality)
	hasPeriods := cfg.Periods()

	return render.PrimeReport{
		SchemaVersion:    render.PrimeSchemaVersion,
		Preset:           cfg.Preset,
		PresetVersion:    cfg.PresetVersion,
		AsOf:             graph.AsOfDay(asOf),
		At:               c.at,
		Dirs:             dirs,
		Kinds:            kinds,
		Edges:            edges,
		Fields:           fields,
		Projections:      cfg.ProjectionNames(),
		RequiredSections: requiredSections,
		HasPeriods:       hasPeriods,
		HasSupersedes:    hasSupersedes,
		HasModality:      hasModality,
		Documents:        summary.Documents,
		Binding:          bindingCount,
		Validation: render.PrimeValidation{
			OK:       summary.Errors == 0,
			Errors:   summary.Errors,
			Warnings: summary.Warnings,
			Cycles:   summary.Cycles,
			Summary:  primeValidationSummary(summary, reportedAsOf(cfg, asOf)),
		},
		SampleRef: sampleRef,
		Commands:  buildPrimeCommands(cfg, fields, hasSupersedes, hasModality),
	}
}

func activeDeclaredFields(cfg config.Config) []string {
	var fields []string
	for _, name := range cfg.DeclaredFields() {
		if !cfg.FieldDeprecated(name) {
			fields = append(fields, name)
		}
	}
	return fields
}

func primeRequiredSections(cfg config.Config) []string {
	if cfg.Sections != nil && len(cfg.Sections.Required) > 0 {
		return slices.Clone(cfg.Sections.Required)
	}
	if !cfg.Multikind() {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, name := range cfg.KindNames() {
		spec, ok := cfg.KindSections(name)
		if !ok {
			continue
		}
		for _, sec := range spec.Required {
			if !seen[sec] {
				seen[sec] = true
				out = append(out, sec)
			}
		}
	}
	return out
}

func exampleWhereField(fields []string) string {
	for _, f := range fields {
		if f != config.FieldModality {
			return f
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return ""
}

func primeValidationSummary(summary model.Summary, asOf string) string {
	var line string
	if summary.Errors == 0 {
		line = fmt.Sprintf("OK: %d docs, %d typed edges, no cycles", summary.Documents, summary.Edges)
	} else {
		line = fmt.Sprintf("FAILED: %d %s", summary.Errors, plural(summary.Errors, "error"))
		if summary.Warnings > 0 {
			line += fmt.Sprintf(", %d %s", summary.Warnings, plural(summary.Warnings, "warning"))
		}
	}
	if asOf != "" {
		line += ", as of " + asOf
	}
	return line
}

func buildPrimeCommands(cfg config.Config, fields []string, hasSupersedes, hasModality bool) []render.PrimeCommand {
	cmds := []render.PrimeCommand{
		{Intent: "context", Command: "docdag context <ref>"},
	}
	if hasSupersedes {
		bindingFields := "id,title,status"
		if hasModality {
			bindingFields = "id,modality,title,status"
		}
		cmds = append(cmds, render.PrimeCommand{
			Intent:  "binding",
			Command: "docdag query --binding --fields " + bindingFields,
		})
	}
	if whereField := exampleWhereField(fields); whereField != "" {
		prefix := "docdag query <ref>"
		if hasSupersedes {
			prefix = "docdag query --binding"
		}
		cmds = append(cmds, render.PrimeCommand{
			Intent:  "filter",
			Command: fmt.Sprintf("%s --where %s=contains:<val> --fields id,title,%s", prefix, whereField, whereField),
		})
	}
	if hasSupersedes {
		cmds = append(cmds, render.PrimeCommand{
			Intent:  "resolve",
			Command: "docdag resolve <ref> --fields id,title,status",
		})
	}
	cmds = append(cmds, render.PrimeCommand{
		Intent:  "impact",
		Command: "docdag query <ref> --ancestors --fields id,title,status",
	})

	newCmd := `docdag new "<title>"`
	if cfg.Multikind() {
		newCmd = fmt.Sprintf(`docdag new "<title>" --kind %s [--id <id>]`, cfg.KindNames()[0])
	} else if hasSupersedes {
		newCmd = `docdag new "<title>" [--supersedes <ref>]`
	}
	cmds = append(cmds,
		render.PrimeCommand{Intent: "new", Command: newCmd},
		render.PrimeCommand{Intent: "validate", Command: "docdag validate --touching <path>"},
		render.PrimeCommand{Intent: "diff", Command: "docdag diff origin/main"},
	)
	return cmds
}
