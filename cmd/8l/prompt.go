package main

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/OneZero1ai/8l-cli/internal/prompts"
)

// newPromptCmd is the `8l prompt …` command tree. The two leaves
// (reflect, skill) emit canonical agent prompts straight from the
// embedded body files. No HTTP, no auth — pure local.
func newPromptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Print a canonical cq agent prompt",
		Long: `Print one of the canonical cq agent prompts for injection into an
agent system prompt. Use this when integrating cq into agent
frameworks that don't have the cq plugin installed.`,
	}
	cmd.AddCommand(newPromptReflectCmd())
	cmd.AddCommand(newPromptSkillCmd())
	return cmd
}

func newPromptReflectCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "reflect",
		Short: "Print the /cq:reflect slash-command prompt",
		RunE: func(cmd *cobra.Command, args []string) error {
			return writePrompt(cmd.OutOrStdout(), format, "prompt", prompts.Reflect())
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	return cmd
}

func newPromptSkillCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the cq agent skill prompt",
		RunE: func(cmd *cobra.Command, args []string) error {
			return writePrompt(cmd.OutOrStdout(), format, "prompt", prompts.Skill())
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Output format: text or json")
	return cmd
}

func writePrompt(w io.Writer, format, jsonKey, body string) error {
	switch format {
	case "text":
		_, err := fmt.Fprint(w, body)
		return err
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{jsonKey: body})
	default:
		return wrapCoded(ExitMissingArg, fmt.Errorf("unsupported format %q: must be text or json", format))
	}
}
