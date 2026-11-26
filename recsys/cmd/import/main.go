package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "import: not implemented (import v3 from local CSV)")
	os.Exit(2)
}
