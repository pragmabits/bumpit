package app

import (
	"encoding/json"
	"fmt"
	"io"
)

func validateOutputFormat(output string) error {
	switch output {
	case "text", "json":
		return nil
	default:
		return fmt.Errorf("unsupported output format %q", output)
	}
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
