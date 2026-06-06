// Rolesetter is a Kubernetes controller that assigns
// node-role.kubernetes.io/<value> labels to nodes based on the value of a
// configurable source label. See the README for configuration and
// deployment details.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mchmarny/rolesetter/pkg/node"
)

// Build metadata injected at link time via GoReleaser ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Fprintf(os.Stdout, "node-role-controller %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	node.InformNodeRoles()
}
