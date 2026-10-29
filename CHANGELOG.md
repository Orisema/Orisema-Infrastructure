# Changelog

Todas as mudanças relevantes deste projeto são registradas neste arquivo. O
formato segue [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o
versionamento usa SemVer com identificadores de pré-lançamento.

## [0.1.0-alpha.4] - Unreleased

### Adicionado

- Primeiro contrato durável para operações multi-etapa, com snapshots
  atômicos, máquina de estados validada e avaliação conservadora de
  interrupções.
- Exclusão de operações ativas por aplicação e listagem determinística,
  falhando fechado diante de qualquer snapshot inválido.
- Comando `operation diagnose` somente leitura para identificar operações
  incompletas e intervenção obrigatória sem consultar o runtime.
- Contrato estruturado de recursos managed/external/unknown e revalidação
  fail-closed de identidade, escopo e ownership.
- Avaliador determinístico de saúde pós-operação com timeout, falhas
  consecutivas e resultados inconclusivos explícitos.
- Persistência atômica da política, observações e avaliação de saúde vinculada
  ao estado `VERIFYING`, com recomputação fail-closed na leitura.

### Limitações

- Ainda não há comando público de update, retomada ou rollback; a avaliação é
  somente leitura e sempre exige intervenção para operações incompletas.

## [0.1.0-alpha.3] - 2026-10-29

### Adicionado

- Especificação e laboratório executável de atualização conservadora,
  verificação de saúde, falha parcial e rollback de aplicação.
- Evidência de preservação de contêiner e volume externos ao alvo.

### Limitações

- O laboratório não é um comando público da CLI e não restaura dados.
- Estados interrompidos exigem inspeção manual até existir journal durável e
  reconciliação específicos para update.

## [0.1.0-alpha.2] - 2026-10-29

### Adicionado

- Contrato `orisema.dev/v1alpha2` com `kind: Application` e migração explícita
  a partir de `v1alpha1`.
- Bind loopback obrigatório e explícito para portas, com comparação de drift.
- Política formal de campos rejeitados, saída humana e exit codes.

### Compatibilidade

- Manifestos `v1alpha1` continuam legíveis sem mudança silenciosa de semântica.
- Environment, secrets, volumes, permissões customizadas e `--output json`
  permanecem fora do contrato e falham fechado.

## [0.1.0-alpha.1] - 2026-10-28

Primeira versão experimental instalável. **Não indicada para produção.**

### Adicionado

- CLI Linux para validar, planejar, criar e inspecionar serviços Podman locais.
- Planejamento conservador com `CREATE`, `NOOP`, `DRIFT` e `REVIEW`.
- Labels de ownership, confirmação explícita e bloqueio de mutações quando há
  dúvida, drift ou conflito de nome.
- Diagnóstico operacional, healthchecks, política limitada de restart e
  recuperação assistida apenas por `START`, com journal e verificação.
- Binários Linux AMD64/ARM64, checksums SHA-256 e comando `orisema version`.
- Quick start reproduzido em Ubuntu 22.04 e 24.04.
- Regressões práticas para conflitos de nome/porta, ownership, cancelamento,
  runtime indisponível e concorrência de recovery.

### Corrigido

- Falhas de `podman run`, como porta ocupada, agora removem o contêiner
  parcialmente criado somente após verificar seus labels de ownership.

### Limitações

- Consulte [Limitações conhecidas](docs/KNOWN_LIMITATIONS.md). Não há update,
  rollback de aplicação/dados, backup/restore, operação remota ou suporte
  comprovado a Docker, macOS ou Windows.

[0.1.0-alpha.1]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.1
[0.1.0-alpha.2]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.2
[0.1.0-alpha.3]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.3
