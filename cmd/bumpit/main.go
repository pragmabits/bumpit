package main

import (
	"os"

	"github.com/pragmabits/bumpit/internal/app"
)

func main() {
	os.Exit(app.Execute(os.Args[1:]))
}
