# Orisema Infrastructure — Alpha release plan

Status: **plano ativo, atualizado em 28/10/2026**. Não representa promessa de
datas nem declara funcionalidade de produção.

## Base existente

- Fase 1 do MVP concluída conforme `ROADMAP.md`: manifesto versionado, CLI
  `validate`/`plan`/`deploy`/`status`, Podman Linux e testes.
- Diagnóstico, healthcheck, restart policy limitada e recuperação START
  assistida com journal e verificação foram implementados.
- Versionamento, empacotamento Linux e documentação de instalação/quick start
  foram entregues pelas Issues #12 e #13.
- O protótipo **não é recomendado para produção**. Não há atualização/rollback
  transacional, backup/restore, suporte Docker comprovado ou operação remota.

## Princípios de lançamento

1. Planejar antes de alterar; falhar fechado em dúvida, drift e ownership.
2. Não confundir comando executado com serviço saudável.
3. Compatibilidade somente quando testada.
4. Release experimental com limitações visíveis, sem datas prometidas.
5. Preservar o contrato atual de manifesto até decisão documentada de migração.

## v0.1.0-alpha.1 — Instalação reproduzível (P0)

- [x] CLI exibe versão, commit e informações de build (#12).
- [x] Pipeline produz binários Linux AMD64 e ARM64 e checksums SHA-256; o
  empacotamento é validado no CI (#12).
- [x] Publicar instruções de instalação, requisitos, quick start e
  desinstalação (#12, #13).
- [x] Documentar versões efetivamente testadas de Go, Linux e Podman; Docker
  permanece não suportado (#13).
- [x] Corrigir `README.md`, `ARCHITECTURE.md` e `PLANNING.md` conforme a
  implementação (#12, #13).
- [x] Adicionar testes de regressão: nome/porta ocupados, ownership,
  DRIFT/REVIEW, indisponibilidade de runtime, confirmação cancelada, recovery
  START e concorrência (#14).
- [x] Configurar private vulnerability reporting e publicar política de suporte
  a versões experimentais (#14).
- [x] Publicar changelog, known limitations e aviso explícito de não produção
  (#14).

**Gate:** instalação a partir de release (sem compilar), quick start
reproduzível em dois ambientes Linux independentes, CI verde, nenhuma mutação
indevida nos cenários negativos e documentação revisada.

**Gate concluído em 28/10/2026:** quick start passou em Ubuntu 22.04.5 com
Podman 3.4.4 e Ubuntu 24.04.5 com Podman 4.9.3. A suíte negativa cobre conflito
de nome/porta, ownership, DRIFT/REVIEW, runtime indisponível, cancelamento e
concorrência de recovery. A tag `v0.1.0-alpha.1` foi publicada como pré-release;
o workflow e uma verificação independente baixaram o artefato AMD64, validaram
seu checksum e executaram `version`, `validate` e `plan` sem compilação local.

## v0.1.0-alpha.2 — Contrato e segurança

- [x] Decidir e documentar evolução de schema com `v1alpha2` e
  `kind: Application`.
- [x] Validar campos desconhecidos e estabelecer estratégia de migração.
- [x] Planejar e testar binds loopback explícitos e segurança de portas.
- [x] Definir env/segredos/volumes/permissões como rejeitados até existir um
  modelo seguro.
- [x] Definir exit codes; adiar saída JSON até existir envelope tipado e seguro.
- [ ] Ampliar testes de concorrência, timeout, interrupção e recuperação.
- [ ] Revisar política de auditoria: operador informado não é identidade
  autenticada; journal local não é tamper-evident.

**Gate:** contrato documentado, testes de falha e segurança passando; nenhuma
promessa de recuperação automática de aplicações unhealthy.

## v0.1.0-alpha.3 — Atualização controlada

- [x] Especificar plano de atualização separado de `CREATE`, com confirmação e
  revalidação.
- [x] Registrar estado anterior suficiente para rollback de aplicação, sem
  prometer rollback de dados.
- [x] Implementar atualização conservadora e verificação pós-mudança em
  laboratório.
- [x] Definir recuperação após falha parcial/interrupção e limites de
  persistência.
- [x] Validar que volumes e contêineres não gerenciados são preservados.

**Gate:** atualização e retorno de versão em laboratório com evidências; limites
de rollback documentados.

**Estado:** gate concluído. As versões `v0.1.0-alpha.2` e
`v0.1.0-alpha.3` são publicadas como pré-releases reproduzíveis, com binários
Linux AMD64/ARM64, checksums e verificação de instalação no CI.

## Beta e futuro (fora do Alpha inicial)

- Matriz ampliada de distribuições/Podman; adaptador Docker somente após testes.
- Backup e restauração efetivamente testados; SSH remoto seguro.
- Observabilidade opcional, multi-node, painel e identidade central somente com
  RFC própria.

## Processo comunitário

- Registrar ADRs: linguagem Go, runtime inicial Podman, falha conservadora,
  separação diagnóstico/recuperação e versionamento de manifesto.
- Definir mantenedores, revisão de PR, convenção de issues, disclosure e
  critérios de release.
- Classificação sugerida: P0 bloqueador, P1 importante, P2 melhoria; áreas
  `cli`, `runtime`, `security`, `docs` e `testing`.

## Verificação antes de anunciar

- Revisar status de PRs e workflow no commit/tag exato.
- Rodar `go test ./...`, `go test -race ./...`, `go vet ./...` e integração
  Podman em ambiente isolado.
- Revisar manualmente artefatos publicados, checksums, instruções e limitações.
- Não anunciar o Alpha enquanto os itens e o gate acima permanecerem pendentes.
