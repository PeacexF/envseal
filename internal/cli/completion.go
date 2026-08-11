package cli

import (
	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/errs"
)

const completionLong = `Print a shell completion script.

  bash        envseal completion bash > /etc/bash_completion.d/envseal
  zsh         envseal completion zsh > "${fpath[1]}/_envseal"
  fish        envseal completion fish > ~/.config/fish/completions/envseal.fish
  powershell  envseal completion powershell | Out-String | Invoke-Expression`

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion <bash|zsh|fish|powershell>",
		Short:     "Print a shell completion script",
		Long:      completionLong,
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},

		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			root := cmd.Root()

			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}

			return errs.New(errs.CodeGeneral, "unsupported shell %q", args[0]).
				Check("choose one of: bash, zsh, fish, powershell")
		},
	}
}
