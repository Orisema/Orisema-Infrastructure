# Changelog

Todas as mudanças relevantes deste projeto são registradas neste arquivo. O
formato segue [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o
versionamento usa SemVer com identificadores de pré-lançamento.

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
