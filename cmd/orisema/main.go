// Copyright 2026 Orisema contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Orisema/Orisema-Infrastructure/internal/backup"
	"github.com/Orisema/Orisema-Infrastructure/internal/buildinfo"
	"github.com/Orisema/Orisema-Infrastructure/internal/diagnostic"
	"github.com/Orisema/Orisema-Infrastructure/internal/manifest"
	"github.com/Orisema/Orisema-Infrastructure/internal/operation"
	"github.com/Orisema/Orisema-Infrastructure/internal/planner"
	"github.com/Orisema/Orisema-Infrastructure/internal/recovery"
	"github.com/Orisema/Orisema-Infrastructure/internal/remote"
	"github.com/Orisema/Orisema-Infrastructure/internal/runtime"
	"github.com/Orisema/Orisema-Infrastructure/internal/updateplan"
)

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprint(stdout, buildinfo.Current().String())
		return 0
	}
	if len(args) > 0 && args[0] == "recover" {
		return runRecoveryPlan(args, stdin, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "operation" {
		return runOperationDiagnosis(args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "update" {
		return runUpdateCommand(args, stdin, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "backup" {
		return runBackupCommand(args, stdin, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "host" {
		return runHostCommand(args, stdout, stderr)
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
		if m.APIVersion == "orisema.dev/v1alpha1" {
			fmt.Fprintln(stdout, "Images may be downloaded; legacy v1alpha1 ports bind to all interfaces. No rollback or health verification.")
		} else {
			fmt.Fprintln(stdout, "Images may be downloaded; v1alpha2 ports use their explicit loopback hostIP. No rollback or health verification.")
		}
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
	fmt.Fprintln(w, "Usage: orisema version OR orisema <validate|plan|status|diagnose|deploy> <manifest.yaml> OR orisema update <plan|prepare|authorize|apply|verify|rollback|rollback-verify> ... OR orisema backup <create|verify|restore> ... OR orisema host check <registry.yaml> <alias> OR orisema <logs|start|stop> <manifest.yaml> <service> OR orisema operation diagnose --journal <directory> OR orisema recover <init|history|plan|prepare|apply|verify|reconcile|record> ...")
}

func runHostCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) != 4 || args[1] != "check" {
		printUsage(stderr)
		return 2
	}
	registry, err := remote.LoadRegistry(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "host check blocked: %v\n", err)
		return 1
	}
	result, err := (remote.Transport{}).Check(context.Background(), *registry, args[3])
	if err != nil {
		fmt.Fprintf(stderr, "host check failed: alias=%s outcome=%s duration=%s\n", args[3], result.Outcome, result.Duration.Round(time.Millisecond))
		return 1
	}
	fmt.Fprintf(stdout, "Host connectivity verified: alias=%s outcome=%s duration=%s\n", result.Host, result.Outcome, result.Duration.Round(time.Millisecond))
	fmt.Fprintln(stdout, "Strict host-key checking and key-only batch authentication were enforced; no remote state was inspected or changed.")
	return 0
}

func runBackupCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 3 && args[1] == "verify" {
		verified, err := backup.Verify(args[2])
		if err != nil {
			fmt.Fprintf(stderr, "backup verification failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Backup verified: %s application=%s entries=%d\n", verified.Metadata.BackupID, verified.Metadata.Application, len(verified.Metadata.Entries))
		fmt.Fprintln(stdout, "Integrity verified; authenticity and persistent application data are not covered.")
		return 0
	}
	create := len(args) == 7 && args[1] == "create" && args[3] == "--output" && args[5] == "--journal"
	createWithHistory := len(args) == 9 && createPrefix(args) && args[7] == "--operations"
	restore := len(args) == 7 && args[1] == "restore" && args[3] == "--destination" && args[5] == "--journal"
	if !create && !createWithHistory && !restore {
		printUsage(stderr)
		return 2
	}
	if create || createWithHistory {
		request := backup.CreateRequest{ManifestPath: args[2], ArtifactPath: args[4], JournalPath: args[6]}
		if createWithHistory {
			request.OperationsPath = args[8]
		}
		meta, err := backup.Create(request)
		if err != nil {
			fmt.Fprintf(stderr, "backup creation failed: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Backup created and verified: %s application=%s path=%s\n", meta.BackupID, meta.Application, args[4])
		fmt.Fprintln(stdout, "Persistent data, volumes, secrets, credentials, images and runtime state were not backed up.")
		return 0
	}
	verified, err := backup.Verify(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "restore blocked: %v\n", err)
		return 1
	}
	confirmation := "RESTORE BACKUP " + verified.Metadata.BackupID + " " + verified.Metadata.Application
	fmt.Fprintf(stdout, "Type %s to restore into a new destination: ", confirmation)
	answer, readErr := bufio.NewReader(stdin).ReadString('\n')
	if readErr != nil || strings.TrimSpace(answer) != confirmation {
		fmt.Fprintln(stderr, "cancelled; journal and destination unchanged")
		return 1
	}
	meta, err := backup.Restore(backup.RestoreRequest{ArtifactPath: args[2], DestinationPath: args[4], JournalPath: args[6]})
	if err != nil {
		fmt.Fprintf(stderr, "restore failed: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Backup restored and verified: %s application=%s destination=%s\n", meta.BackupID, meta.Application, args[4])
	fmt.Fprintln(stdout, "The destination is a configuration/history export only; no runtime operation was resumed.")
	return 0
}

func createPrefix(args []string) bool {
	return len(args) >= 7 && args[1] == "create" && args[3] == "--output" && args[5] == "--journal"
}

func runUpdateCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	planOnly := len(args) == 3 && args[1] == "plan"
	prepare := len(args) == 6 && args[1] == "prepare" && args[4] == "--journal"
	authorize := len(args) == 8 && args[1] == "authorize" && args[4] == "--journal" && args[6] == "--operation"
	apply := len(args) == 8 && args[1] == "apply" && args[4] == "--journal" && args[6] == "--operation"
	verify := len(args) == 8 && args[1] == "verify" && args[4] == "--journal" && args[6] == "--operation"
	rollback := len(args) == 8 && args[1] == "rollback" && args[4] == "--journal" && args[6] == "--operation"
	rollbackVerify := len(args) == 8 && args[1] == "rollback-verify" && args[4] == "--journal" && args[6] == "--operation"
	if !planOnly && !prepare && !authorize && !apply && !verify && !rollback && !rollbackVerify {
		printUsage(stderr)
		return 2
	}
	m, err := manifest.Load(args[2])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if verify {
		return runUpdateVerify(*m, args[3], args[5], args[7], stdout, stderr)
	}
	if rollback {
		return runUpdateRollback(*m, args[3], args[5], args[7], stdin, stdout, stderr)
	}
	if rollbackVerify {
		return runUpdateRollbackVerify(*m, args[3], args[5], args[7], stdout, stderr)
	}
	if authorize {
		service, journal, id := args[3], args[5], args[7]
		readStore, err := operation.OpenExistingStore(journal)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		op, err := readStore.Load(id)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if op.Intent == nil || op.Application != m.Name || op.Intent.Service != service {
			fmt.Fprintln(stderr, "error: operation intent does not match the requested manifest and service")
			return 1
		}
		confirmation := "AUTHORIZE UPDATE " + m.Name + "/" + service
		fmt.Fprintf(stdout, "Type %s to authorize preparation: ", confirmation)
		answer, readErr := bufio.NewReader(stdin).ReadString('\n')
		if readErr != nil || strings.TrimSpace(answer) != confirmation {
			fmt.Fprintln(stderr, "cancelled; journal and runtime unchanged")
			return 1
		}
	}
	if apply {
		service, journal, id := args[3], args[5], args[7]
		readStore, err := operation.OpenExistingStore(journal)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		op, err := readStore.Load(id)
		if err != nil || op.Intent == nil || op.Authorization == nil || op.State != operation.Preparing || op.Application != m.Name || op.Intent.Service != service {
			fmt.Fprintln(stderr, "error: operation is not an authorized PREPARING intent for this manifest and service")
			return 1
		}
		confirmation := "APPLY UPDATE " + m.Name + "/" + service
		fmt.Fprintf(stdout, "Type %s to start runtime mutation: ", confirmation)
		answer, readErr := bufio.NewReader(stdin).ReadString('\n')
		if readErr != nil || strings.TrimSpace(answer) != confirmation {
			fmt.Fprintln(stderr, "cancelled; journal and runtime unchanged")
			return 1
		}
	}
	client := runtime.Client{}
	containers, err := client.List(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	evidence := planner.BuildInspected(context.Background(), m, containers, client)
	steps := updateplan.Build(m, evidence)
	if apply {
		service, journal, id := args[3], args[5], args[7]
		var candidate *updateplan.Step
		var declared *manifest.Service
		for i := range steps {
			if steps[i].Service == service {
				candidate = &steps[i]
				declared = &m.Services[i]
				break
			}
		}
		if candidate == nil || candidate.Action != updateplan.Candidate {
			fmt.Fprintln(stderr, "update execution blocked: fresh plan is no longer an update candidate")
			return 1
		}
		backupName := "orisema-rollback-" + strings.TrimPrefix(id, "update-")
		if _, exists := runtime.Find(containers, backupName); exists {
			fmt.Fprintln(stderr, "update execution blocked: rollback slot already exists")
			return 1
		}
		fingerprint := operation.UpdatePlanFingerprint(m.Name, candidate.Service, candidate.Name, candidate.CurrentID, candidate.CurrentImage, candidate.TargetImage)
		observed := []operation.Resource{{Kind: "container", Name: candidate.Name, ID: candidate.CurrentID, Ownership: operation.OwnershipManaged, Application: m.Name, Service: candidate.Service}}
		store, err := operation.OpenStore(journal)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		if _, err := store.BeginUpdateExecution(id, fingerprint, backupName, observed, time.Time{}); err != nil {
			fmt.Fprintf(stderr, "update execution blocked: %v\n", err)
			return 1
		}
		if err := client.Control(context.Background(), m.Name, service, "stop"); err != nil {
			fmt.Fprintf(stderr, "update stopped before rename: %v; inspect operation %s\n", err, id)
			return 1
		}
		if _, err := store.RecordUpdateExecutionStep(id, "previous-stopped", "", time.Time{}); err != nil {
			fmt.Fprintf(stderr, "journal failed after stopping previous container: %v\n", err)
			return 1
		}
		if _, err := store.PrepareUpdateExecutionStep(id, "rename-previous", time.Time{}); err != nil {
			fmt.Fprintf(stderr, "journal failed before renaming previous container: %v\n", err)
			return 1
		}
		if err := client.RenameContainer(context.Background(), candidate.Name, backupName); err != nil {
			fmt.Fprintf(stderr, "update stopped after preserving previous container state: %v; inspect operation %s\n", err, id)
			return 1
		}
		if _, err := store.RecordUpdateExecutionStep(id, "previous-renamed", "", time.Time{}); err != nil {
			fmt.Fprintf(stderr, "journal failed after renaming previous container: %v\n", err)
			return 1
		}
		if _, err := store.PrepareUpdateExecutionStep(id, "create-candidate", time.Time{}); err != nil {
			fmt.Fprintf(stderr, "journal failed before candidate creation: %v\n", err)
			return 1
		}
		candidateID, err := client.CreateUpdateCandidate(context.Background(), m.Name, *declared)
		if err != nil {
			fmt.Fprintf(stderr, "candidate creation failed; previous container remains preserved as %s: %v\n", backupName, err)
			return 1
		}
		op, err := store.RecordUpdateExecutionStep(id, "candidate-created", candidateID, time.Time{})
		if err != nil {
			fmt.Fprintf(stderr, "journal failed after candidate creation: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Update candidate created: %s state=%s\n", candidateID, op.State)
		fmt.Fprintf(stdout, "Previous container preserved as: %s\n", backupName)
		fmt.Fprintln(stdout, "No health success, cleanup, or rollback has been inferred or executed.")
		return 0
	}
	if authorize {
		service, journal, id := args[3], args[5], args[7]
		var candidate *updateplan.Step
		for i := range steps {
			if steps[i].Service == service {
				candidate = &steps[i]
				break
			}
		}
		if candidate == nil || candidate.Action != updateplan.Candidate {
			reason := "service is not an update candidate"
			if candidate != nil {
				reason = candidate.Reason
			}
			fmt.Fprintf(stderr, "update authorization blocked: %s\n", reason)
			return 1
		}
		fingerprint := operation.UpdatePlanFingerprint(m.Name, candidate.Service, candidate.Name, candidate.CurrentID, candidate.CurrentImage, candidate.TargetImage)
		observed := []operation.Resource{{Kind: "container", Name: candidate.Name, ID: candidate.CurrentID, Ownership: operation.OwnershipManaged, Application: m.Name, Service: candidate.Service}}
		store, err := operation.OpenStore(journal)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		op, err := store.AuthorizeUpdateIntent(id, fingerprint, observed, time.Time{})
		if err != nil {
			fmt.Fprintf(stderr, "update authorization blocked: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Update intent authorized: %s state=%s\n", op.ID, op.State)
		fmt.Fprintln(stdout, "No image pulled or runtime changes made.")
		return 0
	}
	if prepare {
		service := args[3]
		var candidate *updateplan.Step
		for i := range steps {
			if steps[i].Service == service {
				candidate = &steps[i]
				break
			}
		}
		if candidate == nil {
			fmt.Fprintf(stderr, "error: service %q not declared in manifest\n", service)
			return 1
		}
		if candidate.Action != updateplan.Candidate {
			fmt.Fprintf(stderr, "update intent blocked for %s: %s\n", candidate.Name, candidate.Reason)
			return 1
		}
		store, err := operation.OpenStore(args[5])
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		id, err := newOperationID()
		if err != nil {
			fmt.Fprintf(stderr, "error: generate operation ID: %v\n", err)
			return 1
		}
		op, err := store.CreateUpdateIntent(operation.UpdateIntentRequest{
			ID: id, Application: m.Name, Service: candidate.Service, ResourceName: candidate.Name,
			ResourceID: candidate.CurrentID, CurrentImage: candidate.CurrentImage, TargetImage: candidate.TargetImage,
		})
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Update intent persisted: %s\n", op.ID)
		fmt.Fprintf(stdout, "Plan fingerprint: %s\n", op.Intent.PlanFingerprint)
		fmt.Fprintln(stdout, "No confirmation requested, image pulled, or runtime changes made.")
		return 0
	}
	fmt.Fprintf(stdout, "Read-only update plan for %q:\n", m.Name)
	for _, step := range steps {
		fmt.Fprintf(stdout, "  [%s] %s: %s\n", step.Action, step.Name, step.Reason)
		if step.Action == updateplan.Candidate {
			fmt.Fprintf(stdout, "    image: %s -> %s\n", step.CurrentImage, step.TargetImage)
		}
	}
	fmt.Fprintln(stdout, "Candidates are guidance only. No journal created, confirmation requested, or runtime changes made.")
	return 0
}

func runUpdateVerify(m manifest.Manifest, service, journal, id string, stdout, stderr io.Writer) int {
	store, err := operation.OpenStore(journal)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	op, err := store.Load(id)
	if err != nil {
		fmt.Fprintf(stderr, "update verification blocked: load operation journal: %v\n", err)
		return 1
	}
	if op.Intent == nil || op.Execution == nil || op.Application != m.Name || op.Intent.Service != service || (op.State != operation.Applying && op.State != operation.Verifying) {
		fmt.Fprintln(stderr, "update verification blocked: operation is not an APPLYING or VERIFYING update for this manifest and service")
		return 1
	}
	var declared *manifest.Service
	for i := range m.Services {
		if m.Services[i].Name == service {
			declared = &m.Services[i]
			break
		}
	}
	if declared == nil || declared.Healthcheck == nil {
		fmt.Fprintln(stderr, "update verification blocked: declared service healthcheck is required")
		return 1
	}
	client := runtime.Client{}
	containers, err := client.List(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "update verification blocked: list runtime containers: %v\n", err)
		return 1
	}
	configurationMatches := false
	for _, step := range planner.BuildInspected(context.Background(), &m, containers, client) {
		if step.Name == op.IntentResourceName() && step.Action == planner.Noop {
			configurationMatches = true
			break
		}
	}
	if !configurationMatches || declared.Image != op.Intent.TargetImage {
		fmt.Fprintln(stderr, "update verification blocked: candidate no longer matches the declared supported configuration and update intent")
		return 1
	}
	observed, candidate, err := inspectUpdateVerificationResources(context.Background(), client, op)
	if err != nil {
		fmt.Fprintf(stderr, "update verification blocked: %v\n", err)
		return 1
	}
	if op.State == operation.Applying {
		policy, err := updateHealthPolicy(*declared)
		if err != nil {
			fmt.Fprintf(stderr, "update verification blocked: %v\n", err)
			return 1
		}
		op, err = store.StartUpdateHealthVerification(id, policy, observed, time.Time{})
		if err != nil {
			fmt.Fprintf(stderr, "update verification blocked: %v\n", err)
			return 1
		}
	} else if evidence := operation.RevalidateResources(op.Health.VerificationResources, observed); !evidence.Safe {
		fmt.Fprintf(stderr, "update verification blocked: %s\n", firstEvidenceIssue(evidence))
		return 1
	}
	if op.Health != nil && op.Health.Evaluation.Terminal {
		return commitUpdateVerificationDecision(store, client, op, stdout, stderr)
	}
	present, status := candidate.HealthEvidence()
	now := time.Now().UTC()
	observation := operation.HealthObservation{At: now, RuntimeAvailable: true, Running: candidate.State.Running && candidate.State.Status == "running", HealthcheckPresent: present, Status: status}
	op, err = store.RecordHealthObservation(id, observation, now)
	if err != nil {
		fmt.Fprintf(stderr, "update verification blocked: %v\n", err)
		return 1
	}
	evaluation := op.Health.Evaluation
	if !evaluation.Terminal {
		fmt.Fprintf(stdout, "Update health observation: %s state=%s decision=WAIT\n", evaluation.Status, op.State)
		fmt.Fprintf(stdout, "Reason: %s\n", evaluation.Reason)
		fmt.Fprintln(stdout, "No runtime changes, retry, cleanup, or rollback performed.")
		return 0
	}
	return commitUpdateVerificationDecision(store, client, op, stdout, stderr)
}

func commitUpdateVerificationDecision(store *operation.Store, client runtime.Client, op operation.Operation, stdout, stderr io.Writer) int {
	fresh, _, err := inspectUpdateVerificationResources(context.Background(), client, op)
	if err != nil {
		fmt.Fprintf(stderr, "terminal health observed but decision blocked by fresh resource inspection: %v\n", err)
		return 1
	}
	op, err = store.CommitVerificationDecision(op.ID, fresh, time.Time{})
	if err != nil {
		fmt.Fprintf(stderr, "update verification decision blocked: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Update health decision: %s state=%s health=%s\n", op.Decision.Action, op.State, op.Health.Evaluation.Status)
	fmt.Fprintf(stdout, "Reason: %s\n", op.Decision.Reason)
	fmt.Fprintln(stdout, "No runtime changes, cleanup, or rollback performed.")
	return 0
}

func inspectUpdateVerificationResources(ctx context.Context, client runtime.Client, op operation.Operation) ([]operation.Resource, runtime.Inspection, error) {
	if op.Execution == nil || op.Intent == nil || op.IntentResourceName() == "" || op.Execution.CandidateID == "" || op.Execution.PreviousID == "" || op.Execution.BackupName == "" {
		return nil, runtime.Inspection{}, errors.New("update execution evidence is incomplete")
	}
	candidate, err := client.Inspect(ctx, op.IntentResourceName())
	if err != nil {
		return nil, runtime.Inspection{}, fmt.Errorf("inspect candidate: %w", err)
	}
	previous, err := client.Inspect(ctx, op.Execution.BackupName)
	if err != nil {
		return nil, runtime.Inspection{}, fmt.Errorf("inspect preserved previous container: %w", err)
	}
	if candidate.Config.Image != op.Intent.TargetImage {
		return nil, runtime.Inspection{}, errors.New("candidate image no longer matches the update intent")
	}
	if previous.Config.Image != op.Intent.CurrentImage || previous.State.Running {
		return nil, runtime.Inspection{}, errors.New("preserved previous container image or stopped state changed")
	}
	resource := func(name string, item runtime.Inspection) operation.Resource {
		ownership := operation.OwnershipUnknown
		if runtime.Owned(item, op.Application, op.Intent.Service) {
			ownership = operation.OwnershipManaged
		}
		return operation.Resource{Kind: "container", Name: name, ID: item.ID, Ownership: ownership, Application: op.Application, Service: op.Intent.Service}
	}
	return []operation.Resource{resource(op.IntentResourceName(), candidate), resource(op.Execution.BackupName, previous)}, candidate, nil
}

func runUpdateRollback(m manifest.Manifest, service, journal, id string, stdin io.Reader, stdout, stderr io.Writer) int {
	store, err := operation.OpenStore(journal)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	op, err := store.Load(id)
	if err != nil || op.Intent == nil || op.Execution == nil || op.Decision == nil || op.State != operation.RecoveryRequired || op.Application != m.Name || op.Intent.Service != service {
		fmt.Fprintln(stderr, "rollback blocked: operation is not an intact RECOVERY_REQUIRED update for this manifest and service")
		return 1
	}
	confirmation := "ROLLBACK UPDATE " + m.Name + "/" + service + " " + id
	fmt.Fprintf(stdout, "Type %s to restore the preserved application container: ", confirmation)
	answer, readErr := bufio.NewReader(stdin).ReadString('\n')
	if readErr != nil || strings.TrimSpace(answer) != confirmation {
		fmt.Fprintln(stderr, "cancelled; journal and runtime unchanged")
		return 1
	}
	client := runtime.Client{}
	containers, err := client.List(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "rollback blocked: list runtime containers: %v\n", err)
		return 1
	}
	configurationMatches := false
	for _, step := range planner.BuildInspected(context.Background(), &m, containers, client) {
		if step.Name == op.IntentResourceName() && step.Action == planner.Noop {
			configurationMatches = true
			break
		}
	}
	if !configurationMatches {
		fmt.Fprintln(stderr, "rollback blocked: candidate no longer matches the declared supported configuration")
		return 1
	}
	failedName := "orisema-failed-" + strings.TrimPrefix(id, "update-")
	if _, exists := runtime.Find(containers, failedName); exists {
		fmt.Fprintln(stderr, "rollback blocked: failed-candidate diagnostic slot already exists")
		return 1
	}
	observed, _, err := inspectRollbackResources(context.Background(), client, op, false)
	if err != nil {
		fmt.Fprintf(stderr, "rollback blocked: %v\n", err)
		return 1
	}
	if _, err = store.BeginUpdateRollback(id, failedName, observed, time.Time{}); err != nil {
		fmt.Fprintf(stderr, "rollback blocked: %v\n", err)
		return 1
	}
	if err = client.Control(context.Background(), m.Name, service, "stop"); err != nil {
		fmt.Fprintf(stderr, "rollback stopped before candidate preservation: %v; inspect operation %s\n", err, id)
		return 1
	}
	if _, err = store.RecordUpdateRollbackStep(id, "candidate-stopped", time.Time{}); err != nil {
		fmt.Fprintf(stderr, "journal failed after candidate stop: %v\n", err)
		return 1
	}
	if _, err = store.PrepareUpdateRollbackStep(id, "rename-candidate", time.Time{}); err != nil {
		fmt.Fprintf(stderr, "journal failed before candidate rename: %v\n", err)
		return 1
	}
	if err = client.RenameContainer(context.Background(), op.IntentResourceName(), failedName); err != nil {
		fmt.Fprintf(stderr, "rollback stopped while preserving failed candidate: %v; inspect operation %s\n", err, id)
		return 1
	}
	if _, err = store.RecordUpdateRollbackStep(id, "candidate-renamed", time.Time{}); err != nil {
		fmt.Fprintf(stderr, "journal failed after candidate rename: %v\n", err)
		return 1
	}
	if _, err = store.PrepareUpdateRollbackStep(id, "rename-previous", time.Time{}); err != nil {
		fmt.Fprintf(stderr, "journal failed before previous rename: %v\n", err)
		return 1
	}
	if err = client.RenameContainer(context.Background(), op.Execution.BackupName, op.IntentResourceName()); err != nil {
		fmt.Fprintf(stderr, "rollback stopped before restored start: %v; inspect operation %s\n", err, id)
		return 1
	}
	if _, err = store.RecordUpdateRollbackStep(id, "previous-renamed", time.Time{}); err != nil {
		fmt.Fprintf(stderr, "journal failed after previous rename: %v\n", err)
		return 1
	}
	if _, err = store.PrepareUpdateRollbackStep(id, "start-previous", time.Time{}); err != nil {
		fmt.Fprintf(stderr, "journal failed before restored start: %v\n", err)
		return 1
	}
	if err = client.Control(context.Background(), m.Name, service, "start"); err != nil {
		fmt.Fprintf(stderr, "rollback restored the previous name but start failed: %v; inspect operation %s\n", err, id)
		return 1
	}
	op, err = store.RecordUpdateRollbackStep(id, "previous-started", time.Time{})
	if err != nil {
		fmt.Fprintf(stderr, "journal failed after restored start: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Previous application container restored; failed candidate preserved as %s; state=%s\n", failedName, op.State)
	fmt.Fprintln(stdout, "Run update rollback-verify explicitly. No data restore, cleanup, retry, or external-resource mutation was performed.")
	return 0
}

func runUpdateRollbackVerify(m manifest.Manifest, service, journal, id string, stdout, stderr io.Writer) int {
	store, err := operation.OpenStore(journal)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	op, err := store.Load(id)
	if err != nil || op.Intent == nil || op.Rollback == nil || op.Application != m.Name || op.Intent.Service != service || op.State != operation.RollingBack || !op.Rollback.PreviousStarted || op.Rollback.PendingStep != "" {
		fmt.Fprintln(stderr, "rollback verification blocked: rollback is not ready for verification")
		return 1
	}
	client := runtime.Client{}
	containers, err := client.List(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "rollback verification blocked: list runtime containers: %v\n", err)
		return 1
	}
	oldManifest := m
	oldManifest.Services = append([]manifest.Service(nil), m.Services...)
	matchedOldConfiguration := false
	for i := range oldManifest.Services {
		if oldManifest.Services[i].Name == service {
			oldManifest.Services[i].Image = op.Intent.CurrentImage
		}
	}
	for _, step := range planner.BuildInspected(context.Background(), &oldManifest, containers, client) {
		if step.Name == op.IntentResourceName() && step.Action == planner.Noop {
			matchedOldConfiguration = true
			break
		}
	}
	if !matchedOldConfiguration {
		fmt.Fprintln(stderr, "rollback verification blocked: restored container no longer matches its supported previous configuration")
		return 1
	}
	observed, restored, err := inspectRollbackResources(context.Background(), client, op, true)
	if err != nil {
		fmt.Fprintf(stderr, "rollback verification blocked: %v\n", err)
		return 1
	}
	present, status := restored.HealthEvidence()
	if !restored.State.Running || restored.State.Status != "running" {
		status = "not-running"
	}
	if !present {
		status = "no-healthcheck"
	}
	op, err = store.RecordUpdateRollbackHealth(id, observed, status, time.Time{})
	if err != nil {
		fmt.Fprintf(stderr, "rollback verification blocked: %v\n", err)
		return 1
	}
	decision := "WAIT"
	if op.State == operation.RolledBack {
		decision = "ROLLED_BACK"
	}
	fmt.Fprintf(stdout, "Rollback health observation: %s state=%s decision=%s\n", status, op.State, decision)
	fmt.Fprintln(stdout, "No cleanup, data restore, retry, or external-resource mutation was performed.")
	return 0
}

func inspectRollbackResources(ctx context.Context, client runtime.Client, op operation.Operation, restored bool) ([]operation.Resource, runtime.Inspection, error) {
	if op.Intent == nil || op.Execution == nil {
		return nil, runtime.Inspection{}, errors.New("rollback evidence is incomplete")
	}
	candidateName, previousName := op.IntentResourceName(), op.Execution.BackupName
	if restored {
		if op.Rollback == nil {
			return nil, runtime.Inspection{}, errors.New("rollback boundary is missing")
		}
		candidateName, previousName = op.Rollback.FailedName, op.IntentResourceName()
	}
	candidate, err := client.Inspect(ctx, candidateName)
	if err != nil {
		return nil, runtime.Inspection{}, fmt.Errorf("inspect candidate: %w", err)
	}
	previous, err := client.Inspect(ctx, previousName)
	if err != nil {
		return nil, runtime.Inspection{}, fmt.Errorf("inspect preserved previous container: %w", err)
	}
	if candidate.ID != op.Execution.CandidateID || candidate.Config.Image != op.Intent.TargetImage || !runtime.Owned(candidate, op.Application, op.Intent.Service) {
		return nil, runtime.Inspection{}, errors.New("candidate identity, image, or ownership changed")
	}
	if previous.ID != op.Execution.PreviousID || previous.Config.Image != op.Intent.CurrentImage || !runtime.Owned(previous, op.Application, op.Intent.Service) {
		return nil, runtime.Inspection{}, errors.New("previous identity, image, or ownership changed")
	}
	if !restored && previous.State.Running {
		return nil, runtime.Inspection{}, errors.New("preserved previous container is no longer stopped")
	}
	if restored && candidate.State.Running {
		return nil, runtime.Inspection{}, errors.New("failed candidate is no longer stopped")
	}
	resource := func(name string, item runtime.Inspection) operation.Resource {
		return operation.Resource{Kind: "container", Name: name, ID: item.ID, Ownership: operation.OwnershipManaged, Application: op.Application, Service: op.Intent.Service}
	}
	return []operation.Resource{resource(candidateName, candidate), resource(previousName, previous)}, previous, nil
}

func updateHealthPolicy(service manifest.Service) (operation.HealthPolicy, error) {
	interval, err := time.ParseDuration(service.Healthcheck.Interval)
	if err != nil {
		return operation.HealthPolicy{}, err
	}
	probeTimeout, err := time.ParseDuration(service.Healthcheck.Timeout)
	if err != nil {
		return operation.HealthPolicy{}, err
	}
	window := probeTimeout
	for range service.Healthcheck.Retries {
		if window >= 30*time.Minute-interval {
			window = 30 * time.Minute
			break
		}
		window += interval
	}
	failures := service.Healthcheck.Retries
	if failures > 10 {
		failures = 10
	}
	return operation.HealthPolicy{Timeout: window, MaxConsecutiveUnhealthy: failures}, nil
}

func firstEvidenceIssue(evidence operation.ResourceEvidence) string {
	if len(evidence.Issues) == 0 {
		return "resource evidence changed"
	}
	return evidence.Issues[0]
}

func newOperationID() (string, error) {
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return "update-" + hex.EncodeToString(random), nil
}

func runOperationDiagnosis(args []string, stdout, stderr io.Writer) int {
	if len(args) != 4 || args[1] != "diagnose" || args[2] != "--journal" {
		printUsage(stderr)
		return 2
	}
	store, err := operation.OpenExistingStore(args[3])
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	operations, err := store.Incomplete()
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "Incomplete operation diagnosis (read-only):")
	if len(operations) == 0 {
		fmt.Fprintln(stdout, "  none")
	}
	for _, op := range operations {
		assessment := operation.Assess(op)
		fmt.Fprintf(stdout, "  [%s] application=%s type=%s state=%s\n", op.ID, op.Application, op.Type, op.State)
		fmt.Fprintf(stdout, "    services: %s\n", strings.Join(op.Services, ", "))
		fmt.Fprintf(stdout, "    created: %s; updated: %s\n", op.CreatedAt.Format(time.RFC3339), op.UpdatedAt.Format(time.RFC3339))
		fmt.Fprintf(stdout, "    intervention required: %t\n", assessment.Intervention)
		fmt.Fprintf(stdout, "    automatic resume safe: %t\n", assessment.ResumeSafe)
		fmt.Fprintf(stdout, "    rollback candidate: %t\n", assessment.RollbackCandidate)
		fmt.Fprintf(stdout, "    ownership evidence complete: %t (managed=%d; external=%d)\n", assessment.OwnershipComplete, assessment.ManagedResources, assessment.ExternalResources)
		if op.Intent != nil {
			fmt.Fprintf(stdout, "    update intent: service=%s image=%s -> %s\n", op.Intent.Service, op.Intent.CurrentImage, op.Intent.TargetImage)
			fmt.Fprintf(stdout, "    plan fingerprint: %s\n", op.Intent.PlanFingerprint)
			fmt.Fprintf(stdout, "    authorization: %s\n", map[bool]string{true: "revalidated", false: "pending"}[op.Authorization != nil])
		}
		if op.Execution != nil {
			fmt.Fprintf(stdout, "    update execution: previous=%s backup=%s candidate=%s\n", op.Execution.PreviousID, op.Execution.BackupName, op.Execution.CandidateID)
			pending := op.Execution.PendingStep
			if pending == "" {
				pending = "none"
			}
			fmt.Fprintf(stdout, "    pending runtime step: %s\n", pending)
		}
		if op.Health != nil {
			fmt.Fprintf(stdout, "    health verification: status=%s terminal=%t observations=%d\n", op.Health.Evaluation.Status, op.Health.Evaluation.Terminal, len(op.Health.Observations))
			fmt.Fprintf(stdout, "    health reason: %s\n", op.Health.Evaluation.Reason)
		}
		if op.Decision != nil {
			fmt.Fprintf(stdout, "    verification decision: %s -> %s\n", op.Decision.Action, op.Decision.TargetState)
		}
		if op.Rollback != nil {
			pending := op.Rollback.PendingStep
			if pending == "" {
				pending = "none"
			}
			fmt.Fprintf(stdout, "    rollback: failed=%s pending=%s restored-started=%t health=%s\n", op.Rollback.FailedName, pending, op.Rollback.PreviousStarted, op.Rollback.HealthStatus)
		}
		if op.FileOperation != nil {
			pending := op.FileOperation.PendingStep
			if pending == "" {
				pending = "none"
			}
			fmt.Fprintf(stdout, "    file operation: backup=%s artifact=%s destination=%s staging=%s pending=%s\n", op.FileOperation.BackupID, op.FileOperation.Artifact, op.FileOperation.Destination, op.FileOperation.Staging, pending)
		}
		fmt.Fprintf(stdout, "    reason: %s\n", assessment.Reason)
	}
	fmt.Fprintln(stdout, "No runtime inspection or changes made.")
	return 0
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
