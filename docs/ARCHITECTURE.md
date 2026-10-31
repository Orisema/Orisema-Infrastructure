# Arquitetura inicial — proposta

## Problema

Implantações manuais geram erros, atualizações difíceis de auditar e recuperação pouco previsível, especialmente em equipes pequenas.

## Fluxo conceitual

```text
Manifesto YAML
     |
Validação
     |
Plano de mudanças
     |
Executor -> Adaptador Podman -> Host Linux
     |
Estado, verificações e logs
```

## Componentes previstos

- **CLI:** interface de comandos e códigos de retorno.
- **Manifesto:** definição declarativa versionada.
- **Planner:** plano legível sem alterações no host.
- **Executor:** execução controlada do plano aprovado.
- **Provider:** integração com Podman e futuros runtimes.
- **State:** registro das operações sem segredos.
- **Build info:** metadados imutáveis incorporados ao binário durante o build,
  com valores explícitos de desenvolvimento quando não injetados.

## Separação de responsabilidades atual

- **Planner:** compara somente os campos declarativos suportados e produz
  `CREATE`, `NOOP`, `DRIFT` ou `REVIEW`.
- **Diagnostic:** combina o resultado declarativo com execução e saúde
  observadas para recomendar uma ação somente informativa.
- **Recovery plan:** transforma o diagnóstico em ações tipadas, pré-condições e
  riscos, sem possuir capacidade de mutação do runtime.
- **Runtime:** lista e inspeciona contêineres e executa operações explicitamente
  solicitadas pela CLI.
- **Host registry:** valida destinos SSH, caminhos de credenciais externas,
  timeouts e cadeias ProxyJump em arquivo local privado.
- **Remote transport/executor:** gera configuração OpenSSH efêmera, autentica
  chaves e hosts estritamente e expõe somente inspect e logs limitados.
- **Remote authorization:** deriva nomes do manifesto e exige ownership exato;
  não existe API pública de comando remoto arbitrário ou mutação remota.
- **Remote audit:** persiste pares locais `START`/`RESULT`, privados e
  sincronizados, sem credenciais, stderr SSH ou conteúdo de logs.

Saúde não altera o plano declarativo. Diagnóstico não executa recuperação. Essa
separação permite que um contêiner parado, mas equivalente nos campos
declarativos, continue `NOOP` enquanto o diagnóstico informa a falha operacional.
Um plano de recuperação pode recomendar `START`, mas não executa a recomendação.

O journal de operações da Alpha 4, em `internal/operation`, persiste snapshots
versionados por escrita temporária, `fsync`, rename atômico e sincronização do
diretório. Sua máquina de estados rejeita transições inválidas e a avaliação de
operações incompletas é somente leitura. O fluxo Alpha 5 autoriza e executa a
substituição de um serviço, preserva a unidade anterior e liga o candidato à
verificação e decisão duráveis. O rollback manual revalida e restaura somente o
container anterior; não limpa automaticamente nem restaura dados. Consulte
[ALPHA5-UPDATE-PLAN.md](ALPHA5-UPDATE-PLAN.md).

## Fora do escopo inicial

Orquestração multi-node, painel web e gestão central de identidade.

## Distribuição experimental

O pipeline acionado somente por tags compila binários Linux AMD64 e ARM64 com
`CGO_ENABLED=0`, `-trimpath` e metadados via linker. Os arquivos compactados e
checksums são verificados antes da criação da GitHub Release. O comando
`orisema version` apenas lê dados incorporados e informações do runtime Go; não
acessa a rede nem arquivos externos.

## Decisões atuais e abertas

Go 1.23 é a linguagem e versão mínima declarada pelo módulo. Podman em Linux é
o primeiro runtime; Docker, Windows e macOS não são suportados. O contrato atual
é `orisema.dev/v1alpha2`, mantendo leitura compatível de `v1alpha1`; versão,
migração e guardrails estão definidos em
[MANIFEST-V1ALPHA2.md](MANIFEST-V1ALPHA2.md). Gestão de segredos, volumes e um
formato de saída estruturado permanecem deliberadamente fora do contrato. A
matriz entre artefatos produzidos e ambientes realmente executados está em
[COMPATIBILITY.md](COMPATIBILITY.md).

## Backup e restauração

A fronteira aceita em [ADR 0003](adr/0003-safe-backup-boundary.md) separa
configuração, estado operacional e dados persistentes. A implementação atual
protege apenas manifesto validado e exportação opcional de histórico do
journal em artefato privado, versionado, limitado e verificado por SHA-256.
Restore publicará somente em destino ausente e journals importados nunca serão
ativados. Volumes, bancos, secrets e credenciais permanecem excluídos. O fluxo
e seus gates estão em [BACKUP-RESTORE-DESIGN.md](BACKUP-RESTORE-DESIGN.md).

## Operação remota independente

A fronteira aceita em [ADR 0002](adr/0002-independent-connectivity.md) separa
registry, transporte SSH, autenticação externa, executor allowlisted,
autorização por manifesto/ownership, adaptador Podman somente leitura e audit
local. O operador fornece uma rota já funcional, direta ou via bastion. O
Orisema não cria VPN, relay, conta externa nem plano de controle central.
Mutações remotas continuam fora do contrato até existirem autorização,
confirmação, journal no host remoto e recuperação conservadora próprios.
