// Command kimi is the CLI entry point for the kimi-code agent.
package main

import (
	"fmt"
	"os"

	"github.com/visdomtech/kimi-code/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
