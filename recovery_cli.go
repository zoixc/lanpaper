// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"

	"lanpaper/recovery"
)

func runAudit(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("audit", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("root", ".", "Lanpaper working directory")
	jsonOutput := flags.Bool("json", false, "write machine-readable JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	report, err := recovery.Audit(*root)
	if err != nil {
		fmt.Fprintf(stderr, "audit: %v\n", err)
		return 2
	}
	if *jsonOutput {
		_ = json.NewEncoder(stdout).Encode(report)
	} else if report.Healthy() {
		fmt.Fprintln(stdout, "Audit passed: no storage inconsistencies found.")
	} else {
		for _, issue := range report.Issues {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", issue.Code, issue.Path, issue.Detail)
		}
		fmt.Fprintf(stdout, "Found %d issue(s).\n", len(report.Issues))
	}
	if !report.Healthy() {
		return 1
	}
	return 0
}
