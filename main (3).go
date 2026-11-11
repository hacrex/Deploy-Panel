// Command dpctl is the CLI client for the deploypanel REST API. Scaffold only.
package main

import (
	"fmt"
	"os"
)

var version = "0.0.0-dev"

const usage = `dpctl ` + "%s" + `

Usage:
  dpctl login
  dpctl sites list|create|delete
  dpctl deploy <site>
  dpctl logs <site> [-f]
  dpctl backup <site>
  dpctl import <caprover|coolify|cloudpanel|cyberpanel> <path>

(not implemented yet)
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage, version)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "dpctl: %q is not implemented yet\n", os.Args[1])
	os.Exit(1)
}
