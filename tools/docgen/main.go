// docgen is the Capper documentation generator.
// Usage: go run ./tools/docgen <command>
// Commands: check, inventory, markdown, web, pdf, serve, all
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"time"
)

const (
	docsRoot = "docs"
	srcDir   = "docs/src"
	navFile  = "docs/nav.yml"
	cfgFile  = "docs/config.yml"
	distDir  = "docs/dist"
)

// ---- config -----------------------------------------------------------------

type Config struct {
	Site struct {
		Name    string `yaml:"name"`
		Version string `yaml:"version"`
	} `yaml:"site"`
	Outputs struct {
		Markdown struct {
			Enabled bool   `yaml:"enabled"`
			OutDir  string `yaml:"out_dir"`
		} `yaml:"markdown"`
		Web struct {
			Enabled bool   `yaml:"enabled"`
			OutDir  string `yaml:"out_dir"`
		} `yaml:"web"`
		PDF struct {
			Enabled bool `yaml:"enabled"`
			OutDir  string `yaml:"out_dir"`
		} `yaml:"pdf"`
	} `yaml:"outputs"`
	Inventory struct {
		SourceRoots     []string `yaml:"source_roots"`
		RequiredModules []string `yaml:"required_modules"`
	} `yaml:"inventory"`
}

type NavEntry struct {
	Title    string     `yaml:"title"`
	File     string     `yaml:"file,omitempty"`
	Children []NavEntry `yaml:"children,omitempty"`
}

type Nav struct {
	Nav []NavEntry `yaml:"nav"`
}

func loadConfig() (Config, error) {
	var cfg Config
	b, err := os.ReadFile(cfgFile)
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", cfgFile, err)
	}
	return cfg, yaml.Unmarshal(b, &cfg)
}

func loadNav() (Nav, error) {
	var nav Nav
	b, err := os.ReadFile(navFile)
	if err != nil {
		return nav, fmt.Errorf("read %s: %w", navFile, err)
	}
	return nav, yaml.Unmarshal(b, &nav)
}

// ---- front matter -----------------------------------------------------------

type FrontMatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Owner       string   `yaml:"owner"`
	Status      string   `yaml:"status"`
	Reviewed    string   `yaml:"reviewed"`
	Outputs     []string `yaml:"outputs"`
}

var validStatuses = map[string]bool{"draft": true, "review": true, "stable": true, "deprecated": true}

func parseFrontMatter(path string) (FrontMatter, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return FrontMatter{}, "", err
	}
	content := string(b)
	if !strings.HasPrefix(content, "---\n") {
		return FrontMatter{}, content, fmt.Errorf("missing front matter")
	}
	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return FrontMatter{}, content, fmt.Errorf("unclosed front matter")
	}
	rawFM := content[4 : end+4]
	body := content[end+8:]
	var fm FrontMatter
	if err := yaml.Unmarshal([]byte(rawFM), &fm); err != nil {
		return fm, body, fmt.Errorf("parse front matter: %w", err)
	}
	return fm, body, nil
}

// ---- nav file enumeration ---------------------------------------------------

func allNavFiles(entries []NavEntry) []string {
	var out []string
	for _, e := range entries {
		if e.File != "" {
			out = append(out, e.File)
		}
		out = append(out, allNavFiles(e.Children)...)
	}
	return out
}

// ---- check command ----------------------------------------------------------

func cmdCheck() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	_ = cfg

	nav, err := loadNav()
	if err != nil {
		return fmt.Errorf("nav: %w", err)
	}

	var errs []string

	for _, f := range allNavFiles(nav.Nav) {
		full := filepath.Join(srcDir, f)
		if _, err := os.Stat(full); err != nil {
			errs = append(errs, fmt.Sprintf("nav file missing: %s", full))
			continue
		}
		fm, body, err := parseFrontMatter(full)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		if fm.Title == "" {
			errs = append(errs, fmt.Sprintf("%s: missing title in front matter", f))
		}
		if fm.Status != "" && !validStatuses[fm.Status] {
			errs = append(errs, fmt.Sprintf("%s: invalid status %q", f, fm.Status))
		}
		if !strings.Contains(body, "\n# ") && !strings.HasPrefix(body, "# ") {
			errs = append(errs, fmt.Sprintf("%s: missing H1 heading", f))
		}
	}

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "ERROR:", e)
		}
		return fmt.Errorf("%d check error(s)", len(errs))
	}
	fmt.Println("docs-check: OK")
	return nil
}

// ---- inventory command ------------------------------------------------------

type ModuleInfo struct {
	Module string `json:"module"`
	Dir    string `json:"dir"`
	Files  int    `json:"files"`
	Tests  int    `json:"tests"`
}

type RouteInfo struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler"`
	File    string `json:"file"`
}

type TypeInfo struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	File    string `json:"file"`
}

type TestInfo struct {
	Package string `json:"package"`
	File    string `json:"file"`
	Name    string `json:"name"`
}

func scanRoutes(roots []string) []RouteInfo {
	var routes []RouteInfo
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			for _, line := range strings.Split(string(b), "\n") {
				for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
					prefix := "r." + method + "("
					if idx := strings.Index(line, prefix); idx >= 0 {
						rest := line[idx+len(prefix):]
						if end := strings.Index(rest, ","); end > 0 {
							routePath := strings.Trim(rest[:end], `" `)
							handler := ""
							if comma2 := strings.Index(rest[end+1:], ")"); comma2 >= 0 {
								handler = strings.TrimSpace(rest[end+1 : end+1+comma2])
							}
							routes = append(routes, RouteInfo{
								Method: method, Path: routePath,
								Handler: handler, File: path,
							})
						}
					}
				}
			}
			return nil
		})
	}
	return routes
}

func scanTypes(roots []string) []TypeInfo {
	var types []TypeInfo
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			pkg := filepath.Base(filepath.Dir(path))
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "type ") && strings.Contains(line, " struct") {
					parts := strings.Fields(line)
					if len(parts) >= 3 && parts[0] == "type" {
						types = append(types, TypeInfo{Name: parts[1], Package: pkg, File: path})
					}
				}
			}
			return nil
		})
	}
	return types
}

func scanTests(roots []string) []TestInfo {
	var tests []TestInfo
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			pkg := filepath.Base(filepath.Dir(path))
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "func Test") {
					name := line[5:]
					if end := strings.IndexAny(name, "("); end > 0 {
						name = name[:end]
					}
					tests = append(tests, TestInfo{Package: pkg, File: path, Name: name})
				}
			}
			return nil
		})
	}
	return tests
}

func cmdInventory() error {
	if err := os.MkdirAll("docs/generated/inventory", 0o755); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// --- modules ---
	var modules []ModuleInfo
	for _, root := range cfg.Inventory.SourceRoots {
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(root, e.Name())
			goFiles, _ := filepath.Glob(filepath.Join(dir, "*.go"))
			testFiles := 0
			for _, f := range goFiles {
				if strings.HasSuffix(f, "_test.go") {
					testFiles++
				}
			}
			modules = append(modules, ModuleInfo{
				Module: dir, Dir: dir,
				Files: len(goFiles), Tests: testFiles,
			})
		}
	}
	jb, _ := json.MarshalIndent(modules, "", "  ")
	_ = os.WriteFile("docs/generated/inventory/modules.json", jb, 0o644)
	var sb strings.Builder
	sb.WriteString("# Module Inventory\n\n")
	sb.WriteString("| Module | Files | Tests |\n|---|---:|---:|\n")
	for _, m := range modules {
		fmt.Fprintf(&sb, "| `%s` | %d | %d |\n", m.Module, m.Files, m.Tests)
	}
	_ = os.WriteFile("docs/generated/inventory/modules.md", []byte(sb.String()), 0o644)

	// --- routes ---
	routes := scanRoutes(cfg.Inventory.SourceRoots)
	jb, _ = json.MarshalIndent(routes, "", "  ")
	_ = os.WriteFile("docs/generated/inventory/routes.json", jb, 0o644)
	sb.Reset()
	sb.WriteString("# Route Inventory\n\n")
	sb.WriteString("| Method | Path | Handler | File |\n|---|---|---|---|\n")
	for _, r := range routes {
		fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` | `%s` |\n", r.Method, r.Path, r.Handler, r.File)
	}
	_ = os.WriteFile("docs/generated/inventory/routes.md", []byte(sb.String()), 0o644)

	// --- types ---
	types := scanTypes(cfg.Inventory.SourceRoots)
	jb, _ = json.MarshalIndent(types, "", "  ")
	_ = os.WriteFile("docs/generated/inventory/types.json", jb, 0o644)
	sb.Reset()
	sb.WriteString("# Type Inventory\n\n")
	sb.WriteString("| Type | Package | File |\n|---|---|---|\n")
	for _, t := range types {
		fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` |\n", t.Name, t.Package, t.File)
	}
	_ = os.WriteFile("docs/generated/inventory/types.md", []byte(sb.String()), 0o644)

	// --- tests ---
	tests := scanTests(cfg.Inventory.SourceRoots)
	jb, _ = json.MarshalIndent(tests, "", "  ")
	_ = os.WriteFile("docs/generated/inventory/tests.json", jb, 0o644)
	sb.Reset()
	sb.WriteString("# Test Inventory\n\n")
	sb.WriteString("| Package | Test Name | File |\n|---|---|---|\n")
	for _, tt := range tests {
		fmt.Fprintf(&sb, "| `%s` | `%s` | `%s` |\n", tt.Package, tt.Name, tt.File)
	}
	_ = os.WriteFile("docs/generated/inventory/tests.md", []byte(sb.String()), 0o644)

	fmt.Printf("docs-inventory: %d modules, %d routes, %d types, %d tests\n",
		len(modules), len(routes), len(types), len(tests))
	return nil
}

// ---- markdown command -------------------------------------------------------

func cmdMarkdown() error {
	outDir := filepath.Join(distDir, "markdown")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, _ := filepath.Rel(srcDir, path)
		dest := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return copyFile(path, dest)
	})
	if err != nil {
		return err
	}

	// Generate SUMMARY.md
	nav, _ := loadNav()
	var sb strings.Builder
	sb.WriteString("# Summary\n\n")
	writeSummary(&sb, nav.Nav, "")
	if err := os.WriteFile(filepath.Join(outDir, "SUMMARY.md"), []byte(sb.String()), 0o644); err != nil {
		return err
	}

	fmt.Println("docs-markdown: done →", outDir)
	return nil
}

func writeSummary(sb *strings.Builder, entries []NavEntry, indent string) {
	for _, e := range entries {
		if e.File != "" {
			fmt.Fprintf(sb, "%s- [%s](%s)\n", indent, e.Title, e.File)
		} else {
			fmt.Fprintf(sb, "%s- **%s**\n", indent, e.Title)
		}
		writeSummary(sb, e.Children, indent+"  ")
	}
}

// ---- web command ------------------------------------------------------------

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}} — {{.SiteName}}</title>
<style>
body{font-family:system-ui,sans-serif;margin:0;display:flex}
nav{width:240px;min-height:100vh;background:#1a1a2e;color:#eee;padding:1rem;box-sizing:border-box;font-size:.9rem}
nav a{color:#9ec5fe;text-decoration:none;display:block;padding:.2rem 0}
nav a:hover{text-decoration:underline}
main{flex:1;padding:2rem;max-width:860px}
pre{background:#f4f4f4;padding:1rem;overflow:auto}
code{background:#f4f4f4;padding:.1em .3em;border-radius:3px}
</style>
</head>
<body>
<nav>
<strong>{{.SiteName}}</strong>
{{.NavHTML}}
</nav>
<main>
{{.Content}}
</main>
</body>
</html>`

func cmdWeb() error {
	outDir := filepath.Join(distDir, "web")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	cfg, _ := loadConfig()
	nav, _ := loadNav()
	navHTML := renderNavHTML(nav.Nav)

	tmpl := template.Must(template.New("page").Parse(htmlTemplate))

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		fm, body, _ := parseFrontMatter(path)
		rel, _ := filepath.Rel(srcDir, path)
		htmlPath := strings.TrimSuffix(rel, ".md") + ".html"
		dest := filepath.Join(outDir, htmlPath)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}

		title := fm.Title
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(path), ".md")
		}

		f, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer f.Close()
		return tmpl.Execute(f, map[string]any{
			"Title":    title,
			"SiteName": cfg.Site.Name,
			"NavHTML":  template.HTML(navHTML),
			"Content":  template.HTML(markdownToHTML(body)),
		})
	})
	if err != nil {
		return err
	}

	// copy docs/assets → dist/web/assets
	assetsDir := filepath.Join(docsRoot, "assets")
	if info, err2 := os.Stat(assetsDir); err2 == nil && info.IsDir() {
		if cpErr := copyDir(assetsDir, filepath.Join(outDir, "assets")); cpErr != nil {
			fmt.Println("docs-web: warning copying assets:", cpErr)
		}
	}

	// search index
	buildSearchIndex(outDir, srcDir)

	fmt.Println("docs-web: done →", outDir)
	return nil
}

func renderNavHTML(entries []NavEntry) string {
	var sb strings.Builder
	sb.WriteString("<ul style='list-style:none;padding:0;margin:.5rem 0'>")
	for _, e := range entries {
		if e.File != "" {
			fmt.Fprintf(&sb, "<li><a href='/%s'>%s</a></li>", strings.TrimSuffix(e.File, ".md")+".html", e.Title)
		} else {
			fmt.Fprintf(&sb, "<li style='margin-top:.5rem;font-weight:bold;color:#ccc'>%s</li>", e.Title)
		}
		if len(e.Children) > 0 {
			sb.WriteString("<li style='padding-left:1rem'>")
			sb.WriteString(renderNavHTML(e.Children))
			sb.WriteString("</li>")
		}
	}
	sb.WriteString("</ul>")
	return sb.String()
}

// inlineMarkdown converts inline Markdown to HTML (links, bold, code, italic).
func inlineMarkdown(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		// ![alt](url) — image
		if s[i] == '!' && i+1 < len(s) && s[i+1] == '[' {
			if end := strings.Index(s[i+2:], "]("); end >= 0 {
				alt := s[i+2 : i+2+end]
				rest := s[i+2+end+2:]
				if urlEnd := strings.Index(rest, ")"); urlEnd >= 0 {
					url := rest[:urlEnd]
					fmt.Fprintf(&out, `<img src="%s" alt="%s" style="max-width:100%%;border-radius:6px;margin:1rem 0;box-shadow:0 2px 12px rgba(0,0,0,.3)">`,
						template.HTMLEscapeString(url), template.HTMLEscapeString(alt))
					i = i + 2 + end + 2 + urlEnd + 1
					continue
				}
			}
		}
		// [text](url)
		if s[i] == '[' {
			if end := strings.Index(s[i+1:], "]("); end >= 0 {
				text := s[i+1 : i+1+end]
				rest := s[i+1+end+2:]
				if urlEnd := strings.Index(rest, ")"); urlEnd >= 0 {
					url := rest[:urlEnd]
					fmt.Fprintf(&out, `<a href="%s">%s</a>`, template.HTMLEscapeString(url), template.HTMLEscapeString(text))
					i = i + 1 + end + 2 + urlEnd + 1
					continue
				}
			}
		}
		// **bold**
		if i+1 < len(s) && s[i] == '*' && s[i+1] == '*' {
			if end := strings.Index(s[i+2:], "**"); end >= 0 {
				fmt.Fprintf(&out, "<strong>%s</strong>", template.HTMLEscapeString(s[i+2:i+2+end]))
				i = i + 2 + end + 2
				continue
			}
		}
		// `code`
		if s[i] == '`' {
			if end := strings.Index(s[i+1:], "`"); end >= 0 {
				fmt.Fprintf(&out, "<code>%s</code>", template.HTMLEscapeString(s[i+1:i+1+end]))
				i = i + 1 + end + 1
				continue
			}
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func markdownToHTML(md string) string {
	var out strings.Builder
	lines := strings.Split(md, "\n")
	inCode := false
	inPara := false
	inList := false

	flushPara := func() {
		if inPara {
			out.WriteString("</p>\n")
			inPara = false
		}
	}
	flushList := func() {
		if inList {
			out.WriteString("</ul>\n")
			inList = false
		}
	}
	flush := func() {
		flushPara()
		flushList()
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			flush()
			if inCode {
				out.WriteString("</code></pre>\n")
				inCode = false
			} else {
				lang := strings.TrimPrefix(line, "```")
				if lang == "" {
					out.WriteString("<pre><code>")
				} else {
					fmt.Fprintf(&out, `<pre><code class="language-%s">`, template.HTMLEscapeString(lang))
				}
				inCode = true
			}
			continue
		}
		if inCode {
			out.WriteString(template.HTMLEscapeString(line) + "\n")
			continue
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "# "):
			flush()
			fmt.Fprintf(&out, "<h1>%s</h1>\n", inlineMarkdown(line[2:]))
		case strings.HasPrefix(line, "## "):
			flush()
			fmt.Fprintf(&out, "<h2>%s</h2>\n", inlineMarkdown(line[3:]))
		case strings.HasPrefix(line, "### "):
			flush()
			fmt.Fprintf(&out, "<h3>%s</h3>\n", inlineMarkdown(line[4:]))
		case strings.HasPrefix(line, "#### "):
			flush()
			fmt.Fprintf(&out, "<h4>%s</h4>\n", inlineMarkdown(line[5:]))
		case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
			flushPara()
			if !inList {
				out.WriteString("<ul>\n")
				inList = true
			}
			item := trimmed[2:]
			fmt.Fprintf(&out, "<li>%s</li>\n", inlineMarkdown(item))
		case trimmed == "":
			flush()
		default:
			flushList()
			if !inPara {
				out.WriteString("<p>")
				inPara = true
			} else {
				out.WriteString(" ")
			}
			out.WriteString(inlineMarkdown(line))
		}
	}
	flush()
	return out.String()
}

type searchEntry struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Body  string `json:"body"`
}

func buildSearchIndex(outDir, src string) {
	var entries []searchEntry
	_ = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		fm, body, _ := parseFrontMatter(path)
		rel, _ := filepath.Rel(src, path)
		url := "/" + strings.TrimSuffix(rel, ".md") + ".html"
		snippet := body
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		entries = append(entries, searchEntry{Title: fm.Title, URL: url, Body: snippet})
		return nil
	})
	b, _ := json.MarshalIndent(entries, "", "  ")
	_ = os.WriteFile(filepath.Join(outDir, "search-index.json"), b, 0o644)
}

// ---- pdf command ------------------------------------------------------------

type docEntry struct {
	Slug     string
	Title    string
	HTMLPath string // absolute path to the rendered HTML file
}

func collectDocEntries() ([]docEntry, error) {
	webDir := filepath.Join(distDir, "web")
	var entries []docEntry
	err := filepath.Walk(webDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		rel, _ := filepath.Rel(webDir, path)
		slug := strings.TrimSuffix(rel, ".html")
		slug = strings.ReplaceAll(slug, string(filepath.Separator), "-")
		abs, _ := filepath.Abs(path)
		entries = append(entries, docEntry{Slug: slug, HTMLPath: abs})
		return nil
	})
	return entries, err
}

func cmdPDF() error {
	outDir := filepath.Join(distDir, "pdf")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	renderer := findPDFRenderer()
	if renderer == "" {
		htmlDir := filepath.Join(outDir, "html")
		_ = os.MkdirAll(htmlDir, 0o755)
		_ = copyDir(filepath.Join(distDir, "web"), htmlDir)
		fmt.Println("docs-pdf: no PDF renderer found (chromium/google-chrome/wkhtmltopdf).")
		fmt.Println("  Print-ready HTML available at:", htmlDir)
		fmt.Println("  To produce PDFs, install chromium and re-run make docs-pdf")
		return nil
	}

	cfg, _ := loadConfig()

	entries, err := collectDocEntries()
	if err != nil || len(entries) == 0 {
		fmt.Println("docs-pdf: no HTML files found — run make docs-web first")
		return err
	}

	fmt.Printf("docs-pdf: using renderer %s\n", renderer)

	for _, entry := range entries {
		pdfPath := filepath.Join(outDir, entry.Slug+".pdf")
		if renderErr := renderPDF(renderer, entry.HTMLPath, pdfPath); renderErr != nil {
			fmt.Printf("docs-pdf: WARN: failed to render %s: %v\n", entry.Slug, renderErr)
		} else {
			fmt.Printf("docs-pdf: %s → %s\n", entry.Slug, pdfPath)
		}
	}

	// Combined book PDF
	allHTML := make([]string, len(entries))
	for i, e := range entries {
		allHTML[i] = e.HTMLPath
	}
	title := cfg.Site.Name
	if title == "" {
		title = "documentation"
	}
	bookPath := filepath.Join(outDir, title+".pdf")
	if bookErr := renderPDFCombined(renderer, allHTML, bookPath); bookErr != nil {
		fmt.Printf("docs-pdf: WARN: combined book failed: %v\n", bookErr)
	} else {
		fmt.Printf("docs-pdf: combined → %s\n", bookPath)
	}

	fmt.Println("docs-pdf: done →", outDir)
	return nil
}

// renderPDF invokes chromium or wkhtmltopdf to render a single HTML file to PDF.
func renderPDF(renderer, htmlPath, pdfPath string) error {
	switch filepath.Base(renderer) {
	case "chromium", "chromium-browser", "google-chrome":
		return exec.Command(renderer,
			"--headless", "--disable-gpu", "--no-sandbox",
			"--print-to-pdf="+pdfPath,
			"--print-to-pdf-no-header",
			"file://"+htmlPath,
		).Run()
	case "wkhtmltopdf":
		return exec.Command(renderer,
			"--enable-local-file-access",
			"--quiet",
			htmlPath, pdfPath,
		).Run()
	default:
		return fmt.Errorf("unsupported renderer: %s", renderer)
	}
}

// renderPDFCombined produces a single merged PDF from multiple HTML files.
// It builds a temporary wrapper HTML that embeds each page, then renders once.
func renderPDFCombined(renderer string, htmlPaths []string, pdfPath string) error {
	tmp, err := os.CreateTemp("", "capper-docs-*.html")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	fmt.Fprintf(tmp, "<!DOCTYPE html><html><body style='margin:0;padding:0'>\n")
	for _, p := range htmlPaths {
		fmt.Fprintf(tmp,
			`<div style="page-break-after:always"><iframe src="file://%s" style="width:100%%;height:100vh;border:none"></iframe></div>`+"\n",
			p,
		)
	}
	fmt.Fprintf(tmp, "</body></html>\n")
	tmp.Close()

	abs, _ := filepath.Abs(tmp.Name())
	return renderPDF(renderer, abs, pdfPath)
}

func findPDFRenderer() string {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "wkhtmltopdf"} {
		if path, err := findExecutable(name); err == nil {
			return path
		}
	}
	return ""
}

func findExecutable(name string) (string, error) {
	for _, dir := range strings.Split(os.Getenv("PATH"), ":") {
		full := filepath.Join(dir, name)
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}
	return "", fmt.Errorf("not found")
}

// ---- serve command ----------------------------------------------------------

func cmdServe() error {
	webDir := filepath.Join(distDir, "web")
	if _, err := os.Stat(webDir); err != nil {
		return fmt.Errorf("web output not found — run make docs-web first")
	}
	addr := "127.0.0.1:8888"
	// Local docs preview only — loopback bind, no directory listings, TLS with ephemeral cert.
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(webDir, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if strings.Contains(r.URL.Path, "..") {
			http.NotFound(w, r)
			return
		}
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, path)
	})
	cert, err := ephemeralLocalhostCert()
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:      addr,
		Handler:   mux,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	fmt.Println("docs-serve: serving at https://" + addr + " (ephemeral self-signed cert)")
	return srv.ListenAndServeTLS("", "")
}

// ---- all command ------------------------------------------------------------

func cmdAll() error {
	for _, fn := range []func() error{cmdCheck, cmdInventory, cmdMarkdown, cmdWeb, cmdPDF} {
		if err := fn(); err != nil {
			return err
		}
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		dest := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		return copyFile(path, dest)
	})
}

// ---- main -------------------------------------------------------------------

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: docgen <check|inventory|markdown|web|pdf|serve|all>")
		os.Exit(1)
	}

	// Change to repo root (two levels up from tools/docgen).
	if err := os.Chdir(repoRoot()); err != nil {
		fmt.Fprintln(os.Stderr, "chdir:", err)
		os.Exit(1)
	}

	cmds := map[string]func() error{
		"check":     cmdCheck,
		"inventory": cmdInventory,
		"clidocs":   cmdCLIDocs,
		"apidocs":   cmdAPIDocs,
		"markdown":  cmdMarkdown,
		"web":       cmdWeb,
		"pdf":       cmdPDF,
		"serve":     cmdServe,
		"all":       cmdAll,
	}
	fn, ok := cmds[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
	if err := fn(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// repoRoot finds the root by walking up until go.mod is found.
func repoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

// ensure bufio and scanner are used (imported for future use)
var _ = bufio.NewScanner

func ephemeralLocalhostCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	b, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b})
	return tls.X509KeyPair(certPEM, keyPEM)
}
