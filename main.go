package main

import (
	"embed"

	"doc-signer/internal/app"
)

//go:embed templates/*
var templateFS embed.FS

func main() {
	app.RunWithTemplates(templateFS)
}