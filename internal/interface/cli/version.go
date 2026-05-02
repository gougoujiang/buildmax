package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Fprintf(os.Stdout, "buildmax version %s\n", Version)
		},
	}
}
