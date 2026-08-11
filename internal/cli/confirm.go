package cli

import (
	"bufio"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PeacexF/envseal/internal/errs"
)

// confirm asks before an action that reaches outside this machine. Silence is
// never taken as consent: without a terminal and without --yes, the command
// refuses rather than proceeding or hanging on a prompt nobody can answer.
func confirm(cmd *cobra.Command, assumeYes bool, question string) error {
	if assumeYes {
		return nil
	}

	out := cmd.OutOrStdout()
	if !isTerminal(out) {
		return errs.New(errs.CodeGeneral, "cannot ask for confirmation").
			Detailf("This would %s, and there is no terminal to confirm on.", question).
			Check("pass --yes to run without confirmation")
	}

	fmt.Fprintf(out, "\n%s\nContinue? [y/N] ", question)

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return errs.New(errs.CodeGeneral, "cancelled")
	}

	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return errs.New(errs.CodeGeneral, "cancelled")
	}
}
