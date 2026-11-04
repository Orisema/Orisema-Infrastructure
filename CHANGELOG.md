# Changelog

Todas as mudanças relevantes deste projeto são registradas neste arquivo. O
formato segue [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/) e o
versionamento usa SemVer com identificadores de pré-lançamento.

## [Unreleased]

## [0.1.0-alpha.8] - 2026-11-04

### Adicionado

- Intenção de update v2 com evidência canônica e fingerprints de configuração
  pública, secrets e redes sem persistir valores ou locators de providers.
- `update prepare`, `authorize` e `apply` para serviços configurados, incluindo
  preparação journaled de secrets transitórios e criação do candidato com o
  conjunto completo de bindings declarados.
- `update verify`, `rollback` e `rollback-verify` com nova resolução das
  identidades autorizadas, inspeção integral da configuração e restauração do
  mesmo container anterior sem reconstrução.
- Laboratório automatizado de falha configurada até `RECOVERY_REQUIRED` e
  rollback manual saudável até `ROLLED_BACK`.

### Segurança

- Mudanças de identidade de secret, arquivo, rede ou configuração entre fases
  falham fechado antes da decisão ou mutação dependente.
- Recursos externos permanecem fora do lifecycle do update e rollback; nenhum
  valor secreto ou público entra no journal operacional.
- Concorrência na preparação de bindings e no início do rollback admite apenas
  um escritor, com validação adicional sob race detector.

### Validação

- Fedora 45 AMD64 nativo com Podman 6.1.3 rootless, SELinux Enforcing,
  `overlay`/`crun`, secret `file`, configuração pública e rede externa.
- Fedora 43 AMD64 descartável com Podman 5.8.9 para smoke e aplicação do
  candidato configurado.
- MixIdentity artificial isolado com PostgreSQL, Keycloak, backend e nginx:
  falha forçada, restauração do ID anterior e decisão `ROLLED_BACK`.
- Suíte completa com vet, testes, race detector, CodeQL, quickstarts Ubuntu e
  laboratórios Podman automatizados.

### Limitações

- O rollback continua restrito ao container da aplicação; não restaura bancos,
  volumes, migrações ou outros dados persistentes.
- Rotação declarativa de configuração permanece uma substituição futura; uma
  mudança durante uma operação autorizada é bloqueada, nunca aplicada in-place.
- Retenção/remoção do container anterior, aliases DNS, lifecycle de redes,
  secrets por ambiente e update coordenado entre serviços permanecem futuros.

## [0.1.0-alpha.7] - 2026-11-04

### Adicionado

- Planner e CREATE deploy para `v1alpha4`, com configuração explicitamente
  pública e associação a redes Podman externas `bridge` por ID revalidado.
- Labels sem valores para provar o conjunto de ambiente gerenciado e identidades
  de rede, além de laboratório rootless de drift e preservação das redes.
- Laboratório reproduzível com imagens reais de app/web do MixIdentity, bancos
  e Keycloak isolados, credenciais artificiais e fronteira explícita para alias
  DNS ainda não gerenciado.

### Segurança

- Valores públicos não entram em labels, planos ou evidências do laboratório;
  argumentos são enviados ao Podman sem shell e redes externas não recebem
  qualquer operação de lifecycle.

## [0.1.0-alpha.6] - 2026-11-03

### Adicionado

- Contrato `orisema.dev/v1alpha3` com referências externas de segredos e
  bindings por serviço, sem valores inline ou variáveis de ambiente.
- Providers `podman` e `file` com resolução somente de metadados, identidade
  revalidada e entrega somente leitura em `/run/secrets/<target>`.
- Handoff do provider `file` por descritor validado e secret Podman transitório,
  removido após a criação do contêiner sem invalidar reinícios posteriores.
- Ciclo de vida `secret create/list/inspect/remove`, sem comando de exibição ou
  exportação e com confirmação exata, ownership e consumer tracking na remoção.

### Validação

- Laboratórios rootless cobrem entrega, ausência, rotação detectada como drift,
  permissões, links, troca de identidade, limite de tamanho e cleanup.
- Um laboratório isolado entrega uma credencial artificial à imagem real do
  MixIdentity, sem alterar o ambiente de laboratório já existente.
- Canários permanecem ausentes de planos, inspeções, erros e evidências; os
  contratos `v1alpha1` e `v1alpha2` continuam rejeitando campos de segredos.

### Limitações

- Update e rollback permanecem fail-closed quando há bindings de segredos.
- Configuração comum, redes, rotação automatizada e operação remota de segredos
  exigem contratos futuros separados.

## [0.1.0-alpha.5] - 2026-10-31

### Adicionado

- `update plan` somente leitura, que reconhece como candidato apenas drift
  exclusivo de imagem com ownership válido e healthcheck declarado.
- `update prepare`, que repete a inspeção e persiste uma intenção `PENDING`
  vinculada à identidade do contêiner e ao fingerprint determinístico do plano,
  sem modificar o runtime.
- `update authorize`, com confirmação textual exata, validade de cinco minutos
  e revalidação integral antes da transição atômica para `PREPARING`.
- `update apply`, que revalida a intenção autorizada, persiste cada fronteira
  antes da mutação, preserva o contêiner anterior e cria um candidato ainda não
  verificado, sem limpeza ou rollback automáticos.
- `update verify`, que revalida candidato e unidade anterior, persiste
  observações explícitas e decide durablemente `COMPLETED` ou
  `RECOVERY_REQUIRED` somente diante de evidência terminal.
- `update rollback` e `rollback-verify`, com nova confirmação, revalidação de
  identidade/ownership e restauração apenas do contêiner anterior preservado.
- Backup privado, limitado e verificado de manifesto e histórico operacional
  opcional, com restore somente em destino ausente e sem ativar journals.
- Registry SSH local privado, autenticação por chave externa, host keys
  estritas, timeouts, cancelamento e ProxyJump de até três hops.
- `remote status` e `remote logs` somente leitura, com comandos allowlisted,
  ownership exato, limites de saída e audit local durável `START`/`RESULT`.

### Validação

- Laboratórios cobrem update saudável/falho, rollback manual, concorrência,
  interrupção, corrupção e restore sem sobrescrita.
- Topologias reais de `sshd` validam autenticação, host key inválida,
  ProxyJump, rota previamente disponível, inspeção pelo salto e perda do alvo.
- Fluxo local isolado passou com Podman 4.9.3 rootless; Ubuntu 22.04 e 24.04
  permanecem na matriz automatizada. Fedora, CGNAT/VPN real e ARM64 nativo não
  são declarados validados.

### Documentação

- Estado da Alpha 4 reconciliado com a persistência de decisões publicada e
  roadmap estendido para os gates conservadores da Alpha 5.
- Arquitetura, CLI, instalação, compatibilidade, limitações, backup e SSH
  reconciliados com a conclusão da Fase 2.

## [0.1.0-alpha.4] - 2026-10-29

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
- Decisão pura pós-verificação para `WAIT`, `COMPLETE`,
  `RECOVERY_REQUIRED` ou `BLOCKED`, condicionada a evidência fresca de recursos.
- Commit atômico e auditável de decisões elegíveis `COMPLETE` e
  `RECOVERY_REQUIRED`, com recarga sob lock, evidência observada persistida e
  rejeição fail-closed de decisões duplicadas, concorrentes ou adulteradas.

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
[0.1.0-alpha.4]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.4
[0.1.0-alpha.5]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.5
[0.1.0-alpha.6]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.6
[0.1.0-alpha.7]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.7
[0.1.0-alpha.8]: https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.8
