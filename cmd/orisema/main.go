// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/planner"
	"github.com/Orisema/Orisema-Infrastructure/internal/recovery"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "recover" {
		return runRecoveryPlan(args, stdin, stdout, stderr)
	}
	if len(args) < 2 {
		printUsage(stderr)
		return 2
	}
	operation := args[0]
	needsService := operation == "logs" || operation == "start" || operation == "stop"
	if (needsService && len(args) != 3) || (!needsService && (len(args) != 2 || (operation != "validate" && operation != "plan" && operation != "status" && operation != "diagnose" && operation != "deploy"))) {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[1])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if needsService {
		service := args[2]
		found := false
		for _, svc := range m.Services {
			if svc.Name == service {
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(stderr, "error: service %q not declared in manifest\n", service)
			return 1
		}
		client := runtime.Client{}
		if operation == "logs" {
			out, err := client.Logs(context.Background(), m.Name, service)
			if err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return 1
			}
			_, _ = stdout.Write(out)
			return 0
		}
		fmt.Fprintf(stdout, "Operation: %s orisema-%s-%s\n", operation, m.Name, service)
		fmt.Fprintf(stdout, "Type %s to confirm: ", strings.ToUpper(operation))
		answer, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil || strings.TrimSpace(answer) != strings.ToUpper(operation) {
			fmt.Fprintln(stderr, "cancelled")
			return 1
		}
		if err := client.Control(context.Background(), m.Name, service, operation); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s completed for %s\n", operation, service)
		return 0
	}
	switch args[0] {
	case "validate":
		fmt.Fprintf(stdout, "valid: %s (%d services)\n", m.Name, len(m.Services))
	case "plan":
		containers, err := (runtime.Client{}).List(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Read-only Podman plan for %q:\n", m.Name)
		for _, step := range planner.BuildInspected(context.Background(), m, containers, runtime.Client{}) {
			fmt.Fprintf(stdout, "  [%s] %s: %s (%s)\n", step.Action, step.Name, step.Image, step.Reason)
		}
		fmt.Fprintln(stdout, "NOOP checks only ownership labels, configured image reference, ports, healthcheck and restart policy configuration; live health and other settings not verified. No changes made.")
	case "deploy":
		client := runtime.Client{}
		containers, err := client.List(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		steps := planner.BuildInspected(context.Background(), m, containers, client)
		creates := 0
		blocked := false
		fmt.Fprintf(stdout, "Conservative deployment plan for %q:\n", m.Name)
		for _, step := range steps {
			fmt.Fprintf(stdout, "  [%s] %s (%s)\n", step.Action, step.Name, step.Reason)
			switch step.Action {
			case planner.Create:
				creates++
			case planner.Noop:
				// No action: matching declared fields only.
			default:
				blocked = true
			}
		}
		if blocked {
			fmt.Fprintln(stderr, "deployment blocked: DRIFT or REVIEW requires manual investigation; no changes made")
			return 1
		}
		if creates == 0 {
			fmt.Fprintln(stdout, "No containers to create. No changes made. Note: runtime health and undeclared settings are not verified.")
			return 0
		}
		fmt.Fprintln(stdout, "Only CREATE services will be started. Existing containers will not be changed.")
		fmt.Fprintln(stdout, "Images may be downloaded; published ports bind to all interfaces. No rollback or health verification.")
		fmt.Fprint(stdout, "Type DEPLOY to continue: ")
		reader := bufio.NewReader(stdin)
		answer, err := reader.ReadString('\n')
		if err != nil && strings.TrimSpace(answer) != "DEPLOY" {
			fmt.Fprintln(stderr, "cancelled")
			return 1
		}
		if strings.TrimSpace(answer) != "DEPLOY" {
			fmt.Fprintln(stderr, "cancelled")
			return 1
		}
		for i, step := range steps {
			if step.Action != planner.Create {
				continue
			}
			id, err := client.Deploy(context.Background(), m.Name, m.Services[i])
			if err != nil {
				fmt.Fprintf(stderr, "deployment stopped: %v\nPreviously created services, if any, were not rolled back.\n", err)
				return 1
			}
			fmt.Fprintf(stdout, "created %s: %s\n", step.Name, id)
		}
	case "status":
		client := runtime.Client{}
		containers, err := client.List(context.Background())
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Podman status for %q (read-only):\n", m.Name)
		for _, svc := range m.Services {
			name := "orisema-" + m.Name + "-" + svc.Name
			found, ok := runtime.Find(containers, name)
			if !ok {
				fmt.Fprintf(stdout, "  %s: not found\n", name)
				continue
			}
			item, err := client.Inspect(context.Background(), name)
			if err != nil {
				fmt.Fprintf(stdout, "  %s: %s (image: %s; inspect error: %v)\n", name, found.State, found.Image, err)
				continue
			}
			ownership := "unverified"
			if runtime.Owned(item, m.Name, svc.Name) {
				ownership = "verified"
			}
			fmt.Fprintf(stdout, "  %s: %s (image: %s; ownership: %s)\n", name, item.State.Status, item.Config.Image, ownership)
			fmt.Fprintf(stdout, "    running: %t; exit code: %d; restarts: %d\n", item.State.Running, item.State.ExitCode, item.RestartCount)
			if item.State.Running && item.State.StartedAt != "" {
				fmt.Fprintf(stdout, "    started: %s\n", item.State.StartedAt)
			}
			if !item.State.Running && item.State.FinishedAt != "" {
				fmt.Fprintf(stdout, "    finished: %s\n", item.State.FinishedAt)
			}
			fmt.Fprintf(stdout, "    health: %s\n", item.HealthStatus())
		}
	case "diagnose":
		reports, diagnoseErr := diagnostic.Build(context.Background(), m, runtime.Client{})
		fmt.Fprintf(stdout, "Operational diagnosis for %q (read-only):\n", m.Name)
		for _, report := range reports {
			fmt.Fprintf(stdout, "  %s:\n", report.Name)
			fmt.Fprintf(stdout, "    configuration: %s\n", report.Configuration)
			fmt.Fprintf(stdout, "    execution: %s\n", report.Execution)
			fmt.Fprintf(stdout, "    health: %s\n", report.Health)
			fmt.Fprintf(stdout, "    failure: %s\n", report.Failure)
			fmt.Fprintf(stdout, "    recommended action: %s\n", report.Recommendation)
		}
		if diagnoseErr != nil {
			fmt.Fprintln(stderr, "diagnosis incomplete: Podman could not be queried; no changes made")
			return 1
		}

	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: orisema <validate|plan|status|diagnose|deploy> <manifest.yaml> OR orisema <logs|start|stop> <manifest.yaml> <service> OR orisema recover <init|history|plan|prepare|apply|verify|reconcile|record> ...")
}

func runRecoveryPlan(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) >= 2 && args[1] == "record" {
		return runRecoveryRecord(args, stdout, stderr)
	}
	if len(args) >= 2 && args[1] == "prepare" {
		return runRecoveryPrepare(args, stdout, stderr)
	}
	if len(args) >= 2 && args[1] == "init" {
		return runRecoveryInit(args, stdout, stderr)
	}
	if len(args) >= 2 && args[1] == "apply" {
		return runRecoveryApply(args, stdin, stdout, stderr)
	}
	if len(args) >= 2 && args[1] == "history" {
		return runRecoveryHistory(args, stdout, stderr)
	}
	if len(args) >= 2 && args[1] == "reconcile" {
		return runRecoveryReconcile(args, stdin, stdout, stderr)
	}
	if len(args) >= 2 && args[1] == "verify" {
		return runRecoveryVerify(args, stdout, stderr)
	}
	if (len(args) != 3 && len(args) != 5) || args[1] != "plan" || (len(args) == 5 && args[3] != "--history") {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	var attempts []recovery.Attempt
	historyConnected := len(args) == 5
	if historyConnected {
		attempts, err = recovery.LoadHistory(args[4], m.Name)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}
	reports, diagnoseErr := diagnostic.Build(context.Background(), m, runtime.Client{})
	fmt.Fprintf(stdout, "Recovery plan for %q (read-only):\n", m.Name)
	for _, step := range recovery.Build(reports) {
		fmt.Fprintf(stdout, "  [%s] %s\n", step.Action, step.Name)
		fmt.Fprintf(stdout, "    reason: %s\n", step.Reason)
		fmt.Fprintf(stdout, "    risk: %s\n", step.Risk)
		fmt.Fprintf(stdout, "    confirmation required: %t\n", step.RequiresConfirmation)
		fmt.Fprintln(stdout, "    preconditions:")
		for _, condition := range step.Preconditions {
			fmt.Fprintf(stdout, "      - %s\n", condition)
		}
		policy := recovery.PolicyFor(m, step.Service)
		if policy == nil {
			fmt.Fprintln(stdout, "    guardrails: not declared")
		} else {
			fmt.Fprintf(stdout, "    guardrails: max attempts %d; minimum interval %s; cooldown %s\n", policy.MaxAttempts, policy.MinInterval, policy.Cooldown)
		}
		if historyConnected {
			decision := recovery.Evaluate(time.Now(), step.Service, step.Action, policy, attempts)
			fmt.Fprintf(stdout, "    guardrail decision: allowed=%t (%s)\n", decision.Allowed, decision.Reason)
			if !decision.RetryAfter.IsZero() {
				fmt.Fprintf(stdout, "    retry after: %s\n", decision.RetryAfter.Format(time.RFC3339))
			}
			fmt.Fprintln(stdout, "    direct execution eligible: false (recover prepare and a fresh confirmation are required)")
		} else {
			fmt.Fprintln(stdout, "    execution eligible: false (persistent attempt history is not connected)")
		}
	}
	fmt.Fprintln(stdout, "No changes made. Only a separately prepared and explicitly confirmed START can be executed.")
	if diagnoseErr != nil {
		fmt.Fprintln(stderr, "recovery plan incomplete: Podman could not be queried")
		return 1
	}
	return 0
}

func runRecoveryHistory(args []string, stdout, stderr io.Writer) int {
	if len(args) != 5 || args[3] != "--history" {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	records, err := recovery.LoadJournal(args[4], m.Name)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	resolved := make(map[string]bool)
	consumed := make(map[string]bool)
	for _, record := range records {
		if record.Kind == "result" {
			resolved[record.Token] = true
		}
		if record.Kind == "execution" {
			consumed[record.Token] = true
		}
	}
	fmt.Fprintf(stdout, "Recovery journal for %q (read-only):\n", m.Name)
	for _, record := range records {
		switch record.Kind {
		case "journal":
			fmt.Fprintf(stdout, "  %s JOURNAL initialized by %s\n", record.Timestamp.Format(time.RFC3339), record.Operator)
		case "reservation":
			state := "active"
			if consumed[record.Token] {
				state = "consumed"
			} else if !record.ExpiresAt.After(time.Now()) {
				state = "expired"
			}
			fmt.Fprintf(stdout, "  %s RESERVATION %s token=%s state=%s expires=%s\n", record.Timestamp.Format(time.RFC3339), record.Service, shortToken(record.Token), state, record.ExpiresAt.Format(time.RFC3339))
		case "execution":
			state := "unresolved"
			if resolved[record.Token] {
				state = "resolved"
			}
			fmt.Fprintf(stdout, "  %s EXECUTION %s token=%s state=%s\n", record.Timestamp.Format(time.RFC3339), record.Service, shortToken(record.Token), state)
		case "result":
			fmt.Fprintf(stdout, "  %s RESULT %s token=%s status=%s\n", record.Timestamp.Format(time.RFC3339), record.Service, shortToken(record.Token), record.Status)
		case "verification":
			fmt.Fprintf(stdout, "  %s VERIFICATION %s token=%s observation=%s\n", record.Timestamp.Format(time.RFC3339), record.Service, shortToken(record.Token), record.Status)
		default:
			fmt.Fprintf(stdout, "  %s ATTEMPT %s action=%s succeeded=%t\n", record.Timestamp.Format(time.RFC3339), record.Service, record.Action, record.Succeeded)
		}
	}
	unresolved := recovery.UnresolvedExecutions(records)
	fmt.Fprintf(stdout, "Unresolved executions: %d\n", len(unresolved))
	for _, record := range unresolved {
		fmt.Fprintf(stdout, "  token=%s service=%s begun=%s\n", record.Token, record.Service, record.Timestamp.Format(time.RFC3339))
	}
	return 0
}

func runRecoveryVerify(args []string, stdout, stderr io.Writer) int {
	if len(args) != 8 || args[4] != "--history" || args[6] != "--token" {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	service := args[3]
	if recovery.PolicyFor(m, service) == nil {
		fmt.Fprintf(stderr, "error: service %q is missing or has no recovery guardrails\n", service)
		return 1
	}
	reports, diagnoseErr := diagnostic.Build(context.Background(), m, runtime.Client{})
	if diagnoseErr != nil {
		fmt.Fprintln(stderr, "error: verification blocked because Podman could not be queried")
		return 1
	}
	var report *diagnostic.Report
	for i := range reports {
		if reports[i].Service == service {
			report = &reports[i]
			break
		}
	}
	if report == nil {
		fmt.Fprintf(stderr, "error: service %q has no diagnostic report\n", service)
		return 1
	}
	status, err := recovery.VerifyExecution(recovery.VerifyRequest{HistoryPath: args[5], Manifest: m.Name, Service: service, Token: args[7], Report: *report})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Recorded post-command observation for %s: %s. No runtime changes made; causality was not inferred.\n", service, status)
	return 0
}

func runRecoveryReconcile(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 14 || args[4] != "--history" || args[6] != "--token" || args[8] != "--outcome" || args[10] != "--operator" || args[12] != "--reason" || (args[9] != "succeeded" && args[9] != "failed") {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	service := args[3]
	if recovery.PolicyFor(m, service) == nil {
		fmt.Fprintf(stderr, "error: service %q is missing or has no recovery guardrails\n", service)
		return 1
	}
	confirmation := "RECONCILE " + strings.ToUpper(args[9]) + " " + service
	fmt.Fprintf(stdout, "This records an operator assertion; it does not change Podman.\nType %s to continue: ", confirmation)
	answer, readErr := bufio.NewReader(stdin).ReadString('\n')
	if readErr != nil || strings.TrimSpace(answer) != confirmation {
		fmt.Fprintln(stderr, "cancelled; journal was not changed")
		return 1
	}
	reports, diagnoseErr := diagnostic.Build(context.Background(), m, runtime.Client{})
	if diagnoseErr != nil {
		fmt.Fprintln(stderr, "error: reconciliation blocked because Podman could not be queried")
		return 1
	}
	var report *diagnostic.Report
	for i := range reports {
		if reports[i].Service == service {
			report = &reports[i]
			break
		}
	}
	if report == nil {
		fmt.Fprintf(stderr, "error: service %q has no diagnostic report\n", service)
		return 1
	}
	err = recovery.ReconcileExecution(recovery.ReconcileRequest{HistoryPath: args[5], Manifest: m.Name, Service: service, Token: args[7], Outcome: args[9], Operator: args[11], Reason: args[13], Report: *report})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Recorded reconciled-command-%s for %s. No runtime changes made.\n", args[9], service)
	return 0
}

func shortToken(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8] + "..."
}

func runRecoveryApply(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) != 8 || args[4] != "--history" || args[6] != "--token" {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	service := args[3]
	policy := recovery.PolicyFor(m, service)
	if policy == nil {
		fmt.Fprintf(stderr, "error: service %q is missing or has no recovery guardrails\n", service)
		return 1
	}
	confirmation := "APPLY START " + service
	fmt.Fprintf(stdout, "Prepared recovery will start orisema-%s-%s. No rollback is available.\n", m.Name, service)
	fmt.Fprintf(stdout, "Type %s to continue: ", confirmation)
	answer, readErr := bufio.NewReader(stdin).ReadString('\n')
	if readErr != nil || strings.TrimSpace(answer) != confirmation {
		fmt.Fprintln(stderr, "cancelled; reservation was not consumed")
		return 1
	}
	reports, diagnoseErr := diagnostic.Build(context.Background(), m, runtime.Client{})
	if diagnoseErr != nil {
		fmt.Fprintln(stderr, "error: execution blocked because Podman could not be queried; reservation was not consumed")
		return 1
	}
	var report *diagnostic.Report
	for i := range reports {
		if reports[i].Service == service {
			report = &reports[i]
			break
		}
	}
	if report == nil {
		fmt.Fprintf(stderr, "error: service %q has no diagnostic report; reservation was not consumed\n", service)
		return 1
	}
	token := args[7]
	if err := recovery.BeginExecution(recovery.BeginRequest{HistoryPath: args[5], Manifest: m.Name, Token: token, Report: *report, Policy: policy}); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	client := runtime.Client{}
	if err := client.Control(context.Background(), m.Name, service, "start"); err != nil {
		detail := err.Error()
		if len(detail) > 512 {
			detail = detail[:512]
		}
		if recordErr := recovery.CompleteExecution(args[5], token, false, detail, time.Now()); recordErr != nil {
			fmt.Fprintf(stderr, "error: START command failed: %v; additionally, audit result failed: %v\n", err, recordErr)
			return 1
		}
		fmt.Fprintf(stderr, "error: START command failed and was recorded: %v\n", err)
		return 1
	}
	if err := recovery.CompleteExecution(args[5], token, true, "podman start completed", time.Now()); err != nil {
		fmt.Fprintf(stderr, "error: START command completed but its audit result could not be recorded: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "START command completed for %s and was recorded; application recovery is not yet verified.\n", service)
	return 0
}

func runRecoveryInit(args []string, stdout, stderr io.Writer) int {
	if len(args) != 7 || args[3] != "--history" || args[5] != "--operator" {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if err := recovery.InitHistory(args[4], m.Name, args[6], time.Now()); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Initialized recovery history for %q at %s. No runtime changes made.\n", m.Name, args[4])
	return 0
}

func runRecoveryPrepare(args []string, stdout, stderr io.Writer) int {
	if len(args) != 10 || args[4] != "--history" || args[6] != "--operator" || args[8] != "--reason" {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	service := args[3]
	policy := recovery.PolicyFor(m, service)
	if policy == nil {
		fmt.Fprintf(stderr, "error: service %q is missing or has no recovery guardrails\n", service)
		return 1
	}
	reports, diagnoseErr := diagnostic.Build(context.Background(), m, runtime.Client{})
	if diagnoseErr != nil {
		fmt.Fprintln(stderr, "error: preparation blocked because Podman could not be queried")
		return 1
	}
	var report *diagnostic.Report
	for i := range reports {
		if reports[i].Service == service {
			report = &reports[i]
			break
		}
	}
	if report == nil {
		fmt.Fprintf(stderr, "error: service %q has no diagnostic report\n", service)
		return 1
	}
	prepared, err := recovery.Prepare(recovery.PrepareRequest{
		HistoryPath: args[5],
		Manifest:    m.Name,
		Report:      *report,
		Policy:      policy,
		Operator:    args[7],
		Reason:      args[9],
	})
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Prepared START for %s after read-only runtime revalidation.\n", report.Name)
	fmt.Fprintf(stdout, "Reservation token: %s\n", prepared.Token)
	fmt.Fprintf(stdout, "Expires: %s\n", prepared.ExpiresAt.Format(time.RFC3339))
	fmt.Fprintln(stdout, "No runtime changes made. Use recover apply with this token before it expires, then confirm explicitly.")
	return 0
}

func runRecoveryRecord(args []string, stdout, stderr io.Writer) int {
	if len(args) != 8 || args[6] != "--history" || args[4] != "START" || (args[5] != "succeeded" && args[5] != "failed") {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	service := args[3]
	if recovery.PolicyFor(m, service) == nil {
		fmt.Fprintf(stderr, "error: service %q is missing or has no recovery guardrails\n", service)
		return 1
	}
	record := recovery.HistoryRecord{Manifest: m.Name, Service: service, Action: diagnostic.ActionStart, Timestamp: time.Now().UTC(), Succeeded: args[5] == "succeeded"}
	if err := recovery.AppendHistory(args[7], record); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Recorded operator-supplied START result for %s in %s. No runtime changes made.\n", service, args[7])
	return 0
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
