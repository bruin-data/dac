package cmd

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
)

//go:embed skill_templates/create-dashboard/SKILL.md
var createDashboardSkill string

type bundledSkill struct {
	Name        string
	Description string
	ClaudePath  string
	CodexPath   string
	Content     string
}

var dacSkills = []bundledSkill{
	{
		Name:        "create-dashboard",
		Description: "Create and modify DAC dashboards, widgets, filters, queries, and semantic models",
		ClaudePath:  ".claude/skills/create-dashboard/SKILL.md",
		CodexPath:   ".codex/skills/create-dashboard",
		Content:     createDashboardSkill,
	},
}

var skillsDirFlag = &cli.StringFlag{
	Name:    "dir",
	Aliases: []string{"d"},
	Usage:   "Target project directory",
	Value:   ".",
}

func skillsCmd() *cli.Command {
	return &cli.Command{
		Name:  "skills",
		Usage: "List, install, and update DAC agent skills",
		Commands: []*cli.Command{
			skillsListCmd(),
			skillsInstallCmd(),
			skillsUpdateCmd(),
		},
		Action: func(_ context.Context, _ *cli.Command) error {
			return runSkillsList()
		},
	}
}

func skillsListCmd() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List bundled DAC agent skills",
		Action: func(_ context.Context, _ *cli.Command) error {
			return runSkillsList()
		},
	}
}

func skillsInstallCmd() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "Install bundled DAC agent skills into a project",
		ArgsUsage: "[skill...]",
		Flags: []cli.Flag{
			skillsDirFlag,
			&cli.BoolFlag{
				Name:    "force",
				Aliases: []string{"f"},
				Usage:   "Overwrite existing skill files",
			},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return runSkillsInstall(cmd.String("dir"), cmd.Args().Slice(), cmd.Bool("force"))
		},
	}
}

func skillsUpdateCmd() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Update installed DAC agent skills to the bundled version",
		ArgsUsage: "[skill...]",
		Flags: []cli.Flag{
			skillsDirFlag,
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			return runSkillsInstall(cmd.String("dir"), cmd.Args().Slice(), true)
		},
	}
}

func runSkillsList() error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tCLAUDE TARGET\tCODEX TARGET\tDESCRIPTION")
	for _, skill := range dacSkills {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", skill.Name, skill.ClaudePath, skill.CodexPath, skill.Description)
	}
	return w.Flush()
}

func runSkillsInstall(targetDir string, names []string, force bool) error {
	targetAbs, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}
	if info, err := os.Stat(targetAbs); err == nil && !info.IsDir() {
		return fmt.Errorf("skills install target exists and is not a directory: %s", targetDir)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(targetAbs, 0o755); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}

	skills, err := selectSkills(names)
	if err != nil {
		return err
	}

	type skillPlan struct {
		skill          bundledSkill
		claudePath     string
		claudeState    string
		codexPath      string
		codexTargetDir string
		codexState     string
	}

	var plans []skillPlan
	var conflicts []string
	for _, skill := range skills {
		claudePath := filepath.Join(targetAbs, filepath.FromSlash(skill.ClaudePath))
		codexPath := filepath.Join(targetAbs, filepath.FromSlash(skill.CodexPath))
		plan := skillPlan{
			skill:          skill,
			claudePath:     claudePath,
			claudeState:    "install",
			codexPath:      codexPath,
			codexTargetDir: filepath.Dir(claudePath),
			codexState:     "link",
		}
		if existing, err := os.ReadFile(claudePath); err == nil {
			if string(existing) == skill.Content {
				plan.claudeState = "same"
			} else if !force {
				conflicts = append(conflicts, skill.ClaudePath)
			}
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}

		if info, err := os.Lstat(codexPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 && symlinkPointsTo(codexPath, plan.codexTargetDir) {
				plan.codexState = "same"
			} else if !force {
				conflicts = append(conflicts, skill.CodexPath)
			} else {
				plan.codexState = "replace"
			}
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		plans = append(plans, plan)
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("target contains skill files that would be overwritten: %s (use --force to overwrite)", strings.Join(conflicts, ", "))
	}

	for _, plan := range plans {
		if plan.claudeState == "same" {
			fmt.Printf("Already installed %s -> %s\n", plan.skill.Name, plan.claudePath)
		} else {
			if err := os.MkdirAll(filepath.Dir(plan.claudePath), 0o755); err != nil {
				return fmt.Errorf("creating directory for %s: %w", plan.skill.ClaudePath, err)
			}
			if err := os.WriteFile(plan.claudePath, []byte(plan.skill.Content), 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", plan.skill.ClaudePath, err)
			}
			fmt.Printf("Installed %s -> %s\n", plan.skill.Name, plan.claudePath)
		}

		if plan.codexState == "same" {
			fmt.Printf("Already linked %s -> %s\n", plan.skill.Name, plan.codexPath)
			continue
		}
		if plan.codexState == "replace" {
			if err := os.RemoveAll(plan.codexPath); err != nil {
				return fmt.Errorf("removing existing %s: %w", plan.skill.CodexPath, err)
			}
		}
		if err := os.MkdirAll(filepath.Dir(plan.codexPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", plan.skill.CodexPath, err)
		}
		target, err := filepath.Rel(filepath.Dir(plan.codexPath), plan.codexTargetDir)
		if err != nil {
			return fmt.Errorf("resolving symlink target for %s: %w", plan.skill.CodexPath, err)
		}
		if err := os.Symlink(target, plan.codexPath); err != nil {
			return fmt.Errorf("linking %s: %w", plan.skill.CodexPath, err)
		}
		fmt.Printf("Linked %s -> %s\n", plan.skill.Name, plan.codexPath)
	}

	fmt.Println()
	fmt.Println("Restart your agent session to pick up newly installed skills.")
	return nil
}

func selectSkills(names []string) ([]bundledSkill, error) {
	if len(names) == 0 {
		return append([]bundledSkill(nil), dacSkills...), nil
	}

	byName := make(map[string]bundledSkill, len(dacSkills))
	for _, skill := range dacSkills {
		byName[skill.Name] = skill
	}

	selected := make([]bundledSkill, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		skill, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown skill %q (run `dac skills list` to see available skills)", name)
		}
		selected = append(selected, skill)
		seen[name] = true
	}
	return selected, nil
}

func symlinkPointsTo(linkPath, targetPath string) bool {
	linkTarget, err := os.Readlink(linkPath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(linkTarget) {
		linkTarget = filepath.Join(filepath.Dir(linkPath), linkTarget)
	}
	linkAbs, err := filepath.Abs(linkTarget)
	if err != nil {
		return false
	}
	targetAbs, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}
	return filepath.Clean(linkAbs) == filepath.Clean(targetAbs)
}
