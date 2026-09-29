package main

import (
	"os"

	"doc-signer/internal/app"
)

func main() {
	app.RunWithTemplates(os.DirFS("."))
}
