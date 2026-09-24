// Command bumpit computes the next semantic version of a repository from the
// Conventional Commits after its last version tag, and creates that tag
// locally. Run bumpit help for the commands.
package main

import (
	"os"

	"github.com/pragmabits/bumpit/internal/app"
)

func main() {
	os.Exit(app.Execute(os.Args[1:]))
}
