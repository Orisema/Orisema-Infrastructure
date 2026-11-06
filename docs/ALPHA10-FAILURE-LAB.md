# Alpha 10 failure laboratory

This laboratory validates that operational failures preserve evidence, block
unsafe mutation and leave unrelated resources untouched. It never treats a
diagnosis as authorization to resume or clean up an operation.

## Automated matrix

| Scenario | Reproduction | Expected result | Cleanup |
| --- | --- | --- | --- |
| SIGKILL during deploy | `sudo env ORISEMA_BINARY="$PWD/work/orisema" scripts/alpha10-failure-lab.sh` | The journal retains `APPLYING` with a pending service; a fresh process reports `automatic resume safe: false`. | The script kills its shim and unmounts its private tmpfs. |
| Restart/reboot boundary | Run `orisema operation diagnose --journal <persisted-store>` after the interrupted process, or after reboot on a persistent filesystem. | The same incomplete operation is visible; no automatic resume or cleanup occurs. | None; evidence is deliberately retained. |
| Full journal filesystem | The automated lab fills a private 2 MiB tmpfs before a second admission. | Admission fails before `podman run`; the committed operation remains readable after space is released. | The filler is removed and tmpfs unmounted. |
| Corrupt or insecure journal | The lab adds malformed JSON to a copied store and grants group/other directory access to another copy. | Both diagnoses fail closed; permissions and content are not repaired. | Both copies live only in the private tmpfs. |
| Concurrent admission | `go test -race ./internal/operation -run 'TestConcurrent(CreateAllowsOneActiveOperationPerApplication|TransitionAllowsOnlyOneWinner)'` | Exactly one conflicting transition/admission wins. | Go temporary directories are removed automatically. |
| Failed health check | `ORISEMA_HEALTH_COMMAND=false ORISEMA_EXPECT_DECISION=RECOVERY_REQUIRED scripts/alpha5-update-apply-lab.sh` | Verification records `RECOVERY_REQUIRED`; no destructive retry occurs. | The lab removes only its named containers. |
| Occupied port | `scripts/alpha1-negative-regressions.sh` | Deploy requires diagnosis; the port holder retains its identity and the target is absent. | The lab removes only its named containers. |
| SSH loss in read-only flow | `scripts/phase2-remote-inspection-lab.sh` and `scripts/phase2-ssh-transport-lab.sh` | The command fails closed with bounded audit evidence and performs no remote mutation. | The private sshd, user and keys are removed. |

The automated SIGKILL test uses a new process opening the same persistent store,
which is the relevant application-level boundary after a reboot. A physical
reboot can be repeated on a disposable host by placing the journal on persistent
storage, killing power after the shim boundary, booting again and running the
same `operation diagnose` command. CI does not reboot its runner because that
would destroy the evidence collection environment.

## Evidence and secrecy

CI uploads text output and journal metadata from the isolated application. The
fixtures contain no secrets, environment values or credentials. Raw container
configuration is not collected. The sentinel container ID proves that an
unrelated runtime resource survived every injected failure and is removed only
by the final explicit cleanup.
