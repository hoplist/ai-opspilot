package main

import (
	"fmt"
	"io"
)

func inspectCommand(opts globalOptions, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("expected inspect subcommand: service, pod, cluster, or release")
	}
	switch args[0] {
	case "service":
		return runInspectService(opts, args[1:], out)
	case "pod":
		return runInspectPod(opts, args[1:], out)
	case "cluster":
		return runInspectCluster(opts, args[1:], out)
	case "release":
		return runReleaseStatus(opts, args[1:], out)
	default:
		return fmt.Errorf("unknown inspect command: %s", args[0])
	}
}
