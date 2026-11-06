# Orisema Infrastructure — Alpha release plan

Status: **plano ativo, atualizado em 05/11/2026**. Não representa promessa de
datas nem declara funcionalidade de produção.

## Base existente

- Fase 1 do MVP concluída conforme `ROADMAP.md`: manifesto versionado, CLI
  `validate`/`plan`/`deploy`/`status`, Podman Linux e testes.
- Diagnóstico, healthcheck, restart policy limitada e recuperação START
  assistida com journal e verificação foram implementados.
- Versionamento, empacotamento Linux e documentação de instalação/quick start
  foram entregues pelas Issues #12 e #13.
- O protótipo **não é recomendado para produção**. Update/rollback de aplicação,
  backup de configuração e inspeção SSH são conservadores e limitados; não há
  transação de dados, backup de volumes, suporte Docker ou mutação remota.

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
- [x] Ampliar testes de concorrência, timeout, interrupção e recuperação.
- [x] Revisar política de auditoria: operador informado não é identidade
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

## v0.1.0-alpha.4 — Recuperação confiável

- [x] Journal atômico e máquina de estados de operações.
- [x] Exclusão de operação concorrente por aplicação.
- [x] Diagnóstico somente leitura e ownership estruturado.
- [x] Avaliação e persistência determinística de saúde.
- [x] Decisão pós-verificação persistida com evidência revalidada.

**Estado:** gate concluído e pré-release publicada com instalação verificada.
Update e rollback foram conectados posteriormente pelo fluxo Alpha 5.

## v0.1.0-alpha.5 — Operação conservadora

- [x] Classificar candidatos a update sem modificar o runtime (#48).
- [x] Registrar plano e intenção imutáveis antes da execução (#50).
- [x] Revalidar identidade e ownership sob exclusão por aplicação (#52).
- [x] Executar update de um serviço com confirmação explícita (#54).
- [x] Verificar saúde e decidir `COMPLETE` ou `RECOVERY_REQUIRED` (#56).
- [x] Expor rollback manual revalidado, sem prometer restauração de dados (#60).

**Gate:** nenhuma mutação sem journal, confirmação, ownership exato e caminho
de recuperação; interrupções continuam diagnosticáveis e falham fechado.

**Gate concluído em 31/10/2026:** implementação e laboratórios passaram; a
pré-release `v0.1.0-alpha.5` foi publicada no commit `b6d8c7b`, os checksums dos
artefatos AMD64/ARM64 foram verificados e o binário AMD64 baixado confirmou
versão e commit incorporados.

## Fase 2 após Alpha 5 — Backup e operação remota

- [x] Definir fronteira segura, formato e integridade de backup (#62).
- [x] Implementar e testar backup/restore de configuração e histórico (#64).
- [x] Implementar inspeção remota somente leitura sobre o transporte estrito (#68).
- [x] Implementar registry e transporte SSH estrito sem comandos arbitrários (#66).
- [x] Validar ProxyJump real e rota privada previamente disponível (#71).

Backup de dados persistentes permanece fora do escopo até existir um contrato
específico de ownership e consistência.

## v0.1.0-alpha.6 — configuração segura

- [x] Propor o contrato e modelo de ameaça para segredos externos por arquivo
  ([RFC-0002](RFC-0002-secure-secrets.md), #77).
- [x] Implementar a base de `v1alpha3` sem ampliar `v1alpha1` ou `v1alpha2` em
  silêncio (#79); bindings permanecem fail-closed.
- [x] Implementar provider `podman` como padrão, com identidade revalidada,
  entrega por arquivo e interface somente de metadados (#81).
- [x] Implementar provider `file` como compatibilidade com handoff seguro via
  secret Podman transitório e cleanup validado (#83).
- [x] Entregar somente em `/run/secrets/<target>`, sem valor inline ou ambiente.
- [x] Garantir não divulgação em plan, inspect, logs, erros, journal e backup
  para o contrato implementado (#81, #83, #85).
- [x] Validar ausência, rotação, permissões, links, corrida, limites e cleanup
  em Podman rootless.
- [x] Validar uma aplicação real em laboratório com segredos artificiais.

As evidências e a fronteira exata desta conclusão estão registradas na
[auditoria da Alpha 6](ALPHA6-AUDIT.md).

**Gate:** os contratos antigos permanecem compatíveis; nenhum canário aparece
nas evidências; toda identidade é revalidada antes de mutação; falhas deixam o
runtime inalterado ou uma recuperação explícita e diagnosticável. Configuração
não secreta e redes terão contrato próprio e não serão confundidas com esta
entrega.

**Gate concluído em 03/11/2026:** a pré-release `v0.1.0-alpha.6` foi publicada
no commit `92a352c`; o workflow repetiu vet, testes e race detector, publicou
artefatos AMD64/ARM64 e reinstalou com sucesso o binário AMD64 baixado.

## v0.1.0-alpha.7 — configuração pública e redes externas

- [x] Propor contrato, modelo de ameaça e sequência de implementação
  ([RFC-0003](RFC-0003-public-config-and-external-networks.md), #92).
- [x] Adicionar `v1alpha4` com configuração explicitamente pública e referências
  de rede, preservando rejeição nos contratos anteriores (#94).
- [x] Resolver redes Podman externas por identidade e fornecer revalidação para
  o deploy futuro, sem criar, adotar, alterar ou remover redes (#96).
- [x] Detectar drift de ambiente e associações de rede sem imprimir valores
  (#98).
- [x] Validar drift, entrega, cleanup do contêiner e preservação de recursos
  externos em Podman rootless (#98).
- [x] Executar MixIdentity com backend e dependências artificiais isoladas
  (#100; [auditoria](ALPHA7-AUDIT.md)).

**Gate:** nenhuma configuração é tratada como secreta; nenhuma rede externa é
mutada; contratos anteriores permanecem compatíveis; falhas de resolução ou
identidade acontecem antes de criar contêiner.

**Gate concluído em 04/11/2026:** a pré-release `v0.1.0-alpha.7` foi publicada
no commit `af03041`; o workflow repetiu vet, testes e race detector, publicou
artefatos AMD64/ARM64 com checksums verificados e reinstalou com sucesso o
binário AMD64 baixado. Os laboratórios rootless incluem a topologia MixIdentity
isolada.

## v0.1.0-alpha.8 — update e rollback de aplicações configuradas

- [x] Definir intenção v2 e fingerprints sem valores para bindings
  ([RFC-0004](RFC-0004-configured-update-and-rollback.md), #105, #107).
- [x] Resolver e revalidar secrets e redes em plan, prepare e authorize antes
  de habilitar qualquer mutação (#109).
- [x] Criar candidatos com configuração pública, secrets e redes completos,
  revalidando identidades e registrando preparação transitória (#111).
- [x] Integrar verify e rollback à nova evidência sem recriar a unidade anterior
  nem assumir lifecycle de recursos externos (#117).
- [x] Testar rotação entre fases, falha parcial, interrupção, concorrência,
  ausência de divulgação e compatibilidade com updates antigos (#119).
- [x] Executar laboratórios Podman rootless e MixIdentity artificial isolado
  (#119).

**Gate:** nenhuma mutação sem intenção durável, confirmação e revalidação; nenhum
valor secreto ou público no journal; mudanças de identidade falham fechado; a
unidade anterior oferece rollback manual e recursos externos são preservados.

**Gate concluído em 04/11/2026:** intenção v2, execução, verify e rollback
configurados passaram na suíte automatizada, nos laboratórios Podman, na VM
Fedora 45 rootless com SELinux Enforcing e no MixIdentity artificial isolado.
A issue de release #121 foi encerrada pelo PR #122 após todos os checks passarem.
A pré-release `v0.1.0-alpha.8` foi publicada a partir do commit `c12ebb2` e seus
artefatos foram baixados e verificados novamente pelo workflow de release.
Rotação declarativa de configuração permaneceu explicitamente fora da release:
somente drift de imagem é elegível, e mudanças de configuração falham fechado
até uma substituição controlada ser projetada para a Beta.

## v0.1.0-alpha.9 — recuperação operacional e distribuição segura

- [x] Definir diagnóstico unificado, estados recuperáveis e bloqueio de mutação
  ([RFC-0005](RFC-0005-operational-recovery-and-doctor.md), #124, #125).
- [x] Implementar `orisema doctor` sem mutações nem exposição de configuração.
- [x] Bloquear operações conflitantes e recuperar somente estados com evidência
  íntegra, lock, revalidação e confirmação explícita.
- [x] Assinar checksums, produzir provenance e verificar o material publicado
  ([RFC-0006](RFC-0006-signed-distribution-and-installer.md), #135).
- [x] Entregar instalador auditável para destino global ou do usuário, com
  upgrade reversível e desinstalação explícita.
- [x] Executar laboratórios de interrupção e instalação em Ubuntu e Fedora.

**Gate:** uma interrupção comum tem diagnóstico e próximo passo seguros; uma
operação ambígua bloqueia novas mutações sem apagar evidência; artefatos
adulterados ou incompatíveis são rejeitados; falha de upgrade mantém o binário
anterior utilizável.

O gate foi consolidado em 5 de novembro de 2026. A suíte Ubuntu 22.04/24.04,
os laboratórios Podman e o race detector passaram no CI. O laboratório Fedora
45 AMD64 passou em modo rootless, com Podman 6.1.3 e SELinux Enforcing. Consulte
[a auditoria da Alpha 9](ALPHA9-AUDIT.md).

## v0.1.0-alpha.10 — prontidão operacional

- [x] Unificar a admissão e o journal das mutações locais `deploy`, `start` e
  `stop` ([RFC-0007](RFC-0007-unified-local-mutation-journal.md), #145).
- [x] Publicar inventário verificável da CLI, contratos e limitações
  ([inventário da CLI](CLI-INVENTORY.md), #146).
- [x] Exercitar encerramento abrupto, reboot e disco cheio sem perda silenciosa
  de evidência ([laboratório Alpha 10](ALPHA10-FAILURE-LAB.md), #147).
- [x] Tornar os próximos passos de `doctor` acionáveis e ainda somente leitura
  ([contrato do CLI](CLI.md#doctor), #148).
- [ ] Documentar fluxos equivalentes para operadores e agentes, sem atalho de
  segurança (#149).
- [ ] Repetir a matriz de compatibilidade, instalação, upgrade, downgrade e
  artefatos assinados no commit exato da release (#150).

**Gate:** toda mutação local suportada possui intenção durável e admissão
atômica; concorrência e falhas abruptas permanecem diagnosticáveis; documentação
e automação usam o mesmo contrato; upgrade ou downgrade incompatível falha sem
substituir o binário utilizável.

## Integração opcional futura — Orisema Data

O planejamento detalhado está no [roadmap principal](../ROADMAP.md). A integração será precedida por RFC e laboratório com serviço simulado. Não é requisito para as Alphas 1–6; Gera Base fica fora do escopo.

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
