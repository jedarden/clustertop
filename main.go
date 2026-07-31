package main

import (
	"fmt"
	"os"

	"github.com/jedarden/clustertop/internal/syncclusters"
	"github.com/jedarden/clustertop/internal/ui"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "sync-clusters" {
		err = syncclusters.Run(os.Args[2:])
	} else {
		err = ui.Run()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "clustertop:", err)
		os.Exit(1)
	}
}
