# Manual do mantenedor

Este manual é o mapa de manutenção do Orisema Infrastructure na Alpha 10. O
[inventário da CLI](CLI-INVENTORY.md) é a referência de sintaxe e códigos de
saída; este documento explica fronteiras, dependências e evidências.

## Invariantes

- Planejamento e diagnóstico são somente leitura e nunca autorizam mutação.
- Toda mutação local suportada é admitida atomicamente em journal privado antes
  de tocar o runtime; operação incompleta bloqueia concorrentes.
- Identidade e ownership são revalidados na fase que depende deles.
- Falha ou ambiguidade preserva a evidência e fecha o caminho de escrita.
- Secrets não entram em planos, journals, auditoria ou argumentos do processo.
- SSH remoto continua somente leitura e não oferece comando arbitrário.

## Fluxo de um comando

```text
argv -> parser/validação -> manifesto -> resolução de bindings
     -> plano ou diagnóstico (somente leitura)
     -> confirmação explícita
     -> admissão/lock/journal -> adaptador Podman
     -> observação/revalidação -> transição persistida ou recuperação requerida
```

O ponto de entrada é `cmd/orisema`. Regras de domínio ficam em `internal/`;
integrações externas ficam atrás dos adaptadores. Texto de CLI não é API de
máquina. Os contratos duráveis são manifesto versionado e formatos versionados
do journal, backup, histórico de recovery e auditoria remota.

## Mapa dos subsistemas

| Subsistema | Implementação e contrato | Responsabilidade e limite | Evidência principal |
|---|---|---|---|
| CLI | `cmd/orisema`, [inventário](CLI-INVENTORY.md) | valida argv, separa stdout/stderr e códigos; não inventa autorização | testes de `cmd/orisema` |
| Manifesto | `internal/manifest`, [v1alpha4](MANIFEST-V1ALPHA4.md) | decodificação estrita e validação declarativa | testes do manifesto e laboratório de contrato |
| Planner/diagnóstico | `internal/planner`, `internal/diagnostic`, [diagnóstico](OPERATIONAL-DIAGNOSTICS.md) | observa drift/saúde e orienta; nunca muta | testes positivos/negativos e `doctor` |
| Estado/journal | `internal/operation`, [RFC-0007](RFC-0007-unified-local-mutation-journal.md) | admissão, lock, máquina de estados, escrita atômica e recuperação | race detector e [failure lab](ALPHA10-FAILURE-LAB.md) |
| Podman | `internal/runtime`, [runtime](PODMAN.md) | argv estruturado, inspect, deploy e controle local autorizado | testes com executável falso e laboratório Podman |
| Secrets | `internal/secrets`, [RFC-0002](RFC-0002-secure-secrets.md) | resolução tardia e entrega por arquivo; sem valor em evidência | testes de providers e laboratório de secrets |
| Configuração/redes | `internal/networks`, [RFC-0003](RFC-0003-public-config-and-external-networks.md) | configuração pública e redes externas revalidadas; rede não é gerenciada | testes e laboratórios Alpha 7/8 |
| Update/rollback | `internal/operation`, `internal/runtime`, [plano](ALPHA5-UPDATE-PLAN.md) | intenção, autorização, candidato, verificação e rollback duráveis | laboratório de update/rollback |
| Recovery | `internal/recovery`, [planejamento](RECOVERY-PLANNING.md) | ações tipadas, token limitado, histórico privado e reconciliação explícita | testes, failure lab e comandos `recover` |
| Backup/restore | `internal/backup`, [design](BACKUP-RESTORE-DESIGN.md) | somente configuração e histórico opcional; exclui volumes, secrets e dados | laboratório de backup/restore |
| SSH remoto | `internal/remote`, [contrato SSH](SSH.md) | registry privado, host key estrita, allowlist read-only e saída limitada | testes de transporte e laboratórios SSH/ProxyJump |
| Distribuição | workflows, `scripts/`, [instalação](INSTALLATION.md) | build, checksum, Sigstore, attestation e troca atômica do binário | workflow de release e laboratórios de instalação |

## Alterando um contrato

Comece por issue e, quando a fronteira de confiança ou formato persistido
mudar, publique RFC/ADR. Atualize implementação, testes negativos, inventário
da CLI, compatibilidade e limitações no mesmo PR. Formatos persistidos precisam
de versão, rejeição estrita do desconhecido e estratégia explícita de
compatibilidade ou migração.

Para mutações, revise a ordem: validar; observar; admitir/registrar; revalidar;
mutar; persistir resultado. Uma interrupção entre quaisquer dois passos deve
resultar em estado diagnosticável e recuperação conservadora.

## Validação proporcional ao risco

Execute sempre `go test ./...`, `go vet ./...` e `git diff --check`. Use
`go test -race` nos pacotes que tratam journal, arquivos ou concorrência. Uma
mudança de Podman, secrets, backup, update, recovery ou SSH também deve passar
no laboratório correspondente de `.github/workflows/go.yml`. Release exige o
gate separado de compatibilidade e distribuição da issue #150.

Não substitua evidência faltante por expectativa. Registre versão do sistema,
Podman, commit, comandos, códigos de saída e limpeza. Artefatos de CI são
evidência auxiliar; uma aprovação humana continua necessária.

## Incidente e recuperação

Pare novas mutações, preserve journal e saídas, e use primeiro `doctor`,
`operation diagnose` ou `recover operation`. Não edite JSON nem remova locks
manualmente. Aplique apenas a recuperação explicitamente suportada após
revalidar identidade/ownership. Backup do Orisema não é backup de banco ou
volume: restaure dados da aplicação com procedimento próprio.

