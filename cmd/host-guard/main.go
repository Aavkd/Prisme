// host-guard runs on the execution host. SSH keeps its stdin open for the lease.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"prisme.local/platform/internal/coordination"
)

func main() {
	root := flag.String("root", "", "private shared coordination directory")
	target := flag.String("target", "", "configured host target")
	origin := flag.String("origin", "normal", "normal or recovery")
	actor := flag.String("actor", "", "opaque actor")
	reconcile := flag.Bool("reconcile", false, "acknowledge stopped processes and verified runtime")
	epoch := flag.Uint64("epoch", 0, "observed epoch required for reconciliation")
	flag.Parse()
	if *reconcile {
		if coordination.Reconcile(*root, *target, *actor, *epoch) != nil {
			os.Exit(1)
		}
		return
	}
	l, err := coordination.Acquire(*root, *target, *origin, *actor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "host_guard_blocked")
		os.Exit(1)
	}
	_ = json.NewEncoder(os.Stdout).Encode(l.State)
	scan := bufio.NewScanner(os.Stdin)
	scan.Buffer(make([]byte, 128), 256)
	stopped := false
	for scan.Scan() {
		switch scan.Text() {
		case "stopped":
			stopped = true
		case "check":
			if l.Event("check", "active") != nil {
				_ = l.Close(false)
				os.Exit(1)
			}
			fmt.Fprintln(os.Stdout, "ok")
			continue
		default:
		}
		break
	}
	if l.Close(stopped) != nil {
		os.Exit(1)
	}
}
