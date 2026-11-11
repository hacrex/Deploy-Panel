// Command dp-agent runs on every node. It dials the control plane outbound
// (mTLS) and reconciles the node. Scaffold only: see docs/ARCHITECTURE.md.
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "0.0.0-dev"

func main() {
	server := flag.String("server", "", "control plane address, for example panel.example.com:443")
	token := flag.String("enroll-token", "", "one-time enrollment token")
	flag.Parse()

	if *server == "" {
		fmt.Fprintln(os.Stderr, "dp-agent: --server is required")
		os.Exit(2)
	}
	_ = token
	fmt.Printf("dp-agent %s: would connect to %s (not implemented)\n", version, *server)
}
