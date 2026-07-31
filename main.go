// Command docdag extracts a typed directed graph from a directory of Markdown
// documents, enforces DAG invariants on it and answers graph queries.
package main

import (
	"os"

	"github.com/Kaikei-e/DocDag/cmd"
)

func main() {
	os.Exit(cmd.Execute())
}
