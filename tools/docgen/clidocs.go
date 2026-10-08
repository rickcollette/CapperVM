package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"capper/internal/cli"
)

// cmdCLIDocs walks the live `capper` cobra command tree and writes a complete,
// source-accurate CLI reference to docs/src/reference/cli/capper.md. Because it
// introspects the real command objects, every command, subcommand, flag, and
// argument string stays in sync with the binary automatically.
func cmdCLIDocs() error {
	root := cli.NewRootCmd()
	root.InitDefaultHelpFlag()

	var b strings.Builder
	b.WriteString(`---
title: "CLI reference"
description: "Complete capper command tree — every command, subcommand, and flag. Generated from source."
owner: "docs"
status: "stable"
reviewed: "2026-06-13"
outputs:
  - markdown
  - web
  - pdf
---

# CLI reference

`)
	b.WriteString("> Generated from the `capper` command tree by `make docs-cli`. Do not edit by hand.\n\n")
	b.WriteString("Run `capper <command> --help` for the same information at the terminal. ")
	b.WriteString("Global persistent flags apply to every command.\n\n")

	writePersistentFlags(&b, root)

	// Table of contents: top-level groups.
	b.WriteString("## Commands\n\n")
	tops := visibleSubcommands(root)
	for _, c := range tops {
		anchor := anchorFor(c.CommandPath())
		fmt.Fprintf(&b, "- [`%s`](#%s) — %s\n", c.Name(), anchor, oneLine(c.Short))
	}
	b.WriteString("\n")

	for _, c := range tops {
		writeCommand(&b, c, 2)
	}

	out := filepath.Join(srcDir, "reference", "cli", "capper.md")
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Printf("docs-cli: wrote %s (%d top-level groups)\n", out, len(tops))
	return nil
}

// curatedExamples seeds realistic usage for top-level command groups that don't
// set cobra's Example field themselves. Keyed by full command path.
var curatedExamples = map[string]string{
	"capper run": `capper run web.cap --name web-1 --memory 512M --subnet-id <subnet-id> \
  --publish 0.0.0.0:8080:8080/tcp --restart on-failure`,
	"capper compute":  "capper compute group create web --template web-tmpl --desired 3 --min 2 --max 10",
	"capper network":  "capper vpc create prod --cidr 10.0.0.0/16 --home-region local",
	"capper vpc":      "capper vpc create prod --cidr 10.0.0.0/16 --home-region local",
	"capper lb":       "capper lb create web-lb --listen 0.0.0.0:8080 --mode http --select tier=web",
	"capper storage":  "capper storage volume create data --size 20G --class local --encrypted",
	"capper iam":      "capper iam user create alice --local-user alice",
	"capper secret":   `capper secret create db-password --value "s3cr3t"`,
	"capper kms":      "capper kms key create app-key",
	"capper cert":     "capper cert issue --cn svc.internal --san DNS:svc.internal",
	"capper dns":      "capper dns zone create internal.example.",
	"capper firewall": "capper firewall rule ... && capper firewall apply",
	"capper backup":   "capper backup policy-create --schedule '@daily' --retain 7 --resource <id>",
	"capper org":      "capper org create acme && capper org account-create --org acme prod",
	"capper node":     "capper node join my-node --token <join-token> --address 10.0.0.5 --role compute",
	"capper aio":      "capper aio init --backend capdb && capper aio up",
}

// writeCommand renders a command and recurses into its subcommands. level is the
// markdown heading level (capped so deep trees stay readable).
func writeCommand(b *strings.Builder, c *cobra.Command, level int) {
	hashes := strings.Repeat("#", min(level, 6))
	fmt.Fprintf(b, "%s `%s`\n\n", hashes, c.CommandPath())
	if s := oneLine(c.Short); s != "" {
		fmt.Fprintf(b, "%s\n\n", s)
	}
	if long := strings.TrimSpace(c.Long); long != "" && long != strings.TrimSpace(c.Short) {
		fmt.Fprintf(b, "%s\n\n", long)
	}
	if c.Runnable() {
		fmt.Fprintf(b, "```text\n%s\n```\n\n", strings.TrimSpace(c.UseLine()))
	}
	// Prefer the command's own Example; otherwise fall back to a curated example
	// for the top-level group so the reference shows real usage, not just flags.
	ex := strings.TrimSpace(c.Example)
	if ex == "" {
		ex = curatedExamples[c.CommandPath()]
	}
	if ex != "" {
		fmt.Fprintf(b, "Example:\n\n```bash\n%s\n```\n\n", ex)
	}
	writeLocalFlags(b, c)

	subs := visibleSubcommands(c)
	if len(subs) > 0 {
		fmt.Fprintf(b, "**Subcommands:** ")
		names := make([]string, len(subs))
		for i, s := range subs {
			names[i] = "`" + s.Name() + "`"
		}
		b.WriteString(strings.Join(names, " · ") + "\n\n")
		for _, s := range subs {
			writeCommand(b, s, level+1)
		}
	}
}

func writePersistentFlags(b *strings.Builder, root *cobra.Command) {
	b.WriteString("## Global flags\n\n")
	b.WriteString("| Flag | Default | Description |\n| --- | --- | --- |\n")
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", flagName(f), flagDefault(f), oneLine(f.Usage))
	})
	b.WriteString("\n")
}

func writeLocalFlags(b *strings.Builder, c *cobra.Command) {
	fl := c.LocalNonPersistentFlags()
	if !fl.HasFlags() {
		return
	}
	b.WriteString("| Flag | Default | Description |\n| --- | --- | --- |\n")
	fl.VisitAll(func(f *pflag.Flag) {
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", flagName(f), flagDefault(f), oneLine(f.Usage))
	})
	b.WriteString("\n")
}

func visibleSubcommands(c *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, s := range c.Commands() {
		if s.Hidden || s.Name() == "help" || s.Name() == "completion" {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func anchorFor(path string) string {
	a := strings.ToLower(path)
	a = strings.ReplaceAll(a, " ", "-")
	return strings.ReplaceAll(a, "_", "-")
}

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	return strings.ReplaceAll(s, "|", "\\|")
}

// flagName renders a flag as `--name, -x` (shorthand only when present).
func flagName(f *pflag.Flag) string {
	if f.Shorthand != "" {
		return fmt.Sprintf("--%s, -%s", f.Name, f.Shorthand)
	}
	return "--" + f.Name
}

// flagDefault renders a flag's default, using an em dash for empty/false zero
// values so the table reads cleanly.
func flagDefault(f *pflag.Flag) string {
	switch f.DefValue {
	case "", "false", "0", "[]":
		return "—"
	}
	return "`" + oneLine(f.DefValue) + "`"
}
