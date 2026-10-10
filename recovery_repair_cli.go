// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"lanpaper/recovery"
)

func runRepair(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("repair", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "Lanpaper working directory")
	dry := flags.Bool("dry-run", false, "print the repair plan without changing files")
	apply := flags.Bool("apply", false, "apply the repair plan offline")
	jsonOutput := flags.Bool("json", false, "write machine-readable output")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *dry == *apply {
		fmt.Fprintln(stderr, "repair: choose exactly one of --dry-run or --apply")
		return 2
	}
	plan, err := recovery.PlanRepair(*root)
	if err != nil {
		fmt.Fprintf(stderr, "repair: %v\n", err)
		return 2
	}
	if *jsonOutput {
		_ = json.NewEncoder(stdout).Encode(plan)
	} else {
		for _, op := range plan.Operations {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", op.Action, op.Path, op.Detail)
		}
		for _, issue := range plan.Manual {
			fmt.Fprintf(stdout, "manual\t%s\t%s\n", issue.Path, issue.Detail)
		}
		fmt.Fprintf(stdout, "Planned %d operation(s); %d issue(s) require manual recovery.\n", len(plan.Operations), len(plan.Manual))
	}
	if *apply {
		if err := recovery.ApplyRepair(plan); err != nil {
			fmt.Fprintf(stderr, "repair: %v\n", err)
			return 2
		}
		fmt.Fprintln(stdout, "Repair applied. Run audit again to verify storage.")
	}
	return 0
}
