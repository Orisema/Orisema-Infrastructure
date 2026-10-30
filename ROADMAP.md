# Roadmap

Etapas propostas, sem datas prometidas.

O plano verificável para as versões experimentais está em
[docs/ALPHA_ROADMAP.md](docs/ALPHA_ROADMAP.md). As Alphas 1–4 foram publicadas;
a Alpha 5 começa pela exposição somente leitura do plano de atualização.

## Fase 0 — Fundação
- [x] Criar organização e repositório públicos.
- [x] Publicar manifesto e documentação inicial.
- [x] Escolher licença open source (Apache-2.0).
- [x] Definir mantenedores e decisões arquiteturais (ver [GOVERNANCE.md](GOVERNANCE.md) e [ADR 0001](docs/adr/0001-initial-governance.md)).

## Fase 1 — MVP
- [x] Definir esquema de manifesto YAML versionado.
- [x] Implementar comandos `validate`, `plan`, `deploy` e `status`.
- [x] Suportar Podman em um host Linux.
- [x] Adicionar testes automatizados e ambiente descartável de integração.

## Fase 2 — Operação
- [ ] Backup e restauração testados.
- [ ] Atualização controlada e rollback.
- [x] Diagnóstico e verificações de saúde.
- [ ] Operação remota segura por SSH.

### Alpha 4 — recuperação confiável

- [x] Auditar o journal `START` existente e preservar seu contrato.
- [x] Introduzir snapshots duráveis e uma máquina de estados para operações.
- [x] Detectar conflitos entre operações da mesma aplicação.
- [x] Integrar diagnóstico somente leitura à CLI.
- [x] Estruturar e revalidar evidências de ownership e recursos.
- [x] Definir verificação objetiva de saúde com timeout e falhas consecutivas.
- [x] Persistir política, observações e avaliações de saúde em `VERIFYING`.
- [x] Definir e persistir atomicamente a decisão pós-verificação elegível.
- [ ] Conectar update/rollback somente após revalidação de ownership, journal
  de intenção e confirmação explícita.

Esta infraestrutura não executa retomada, update ou rollback. Operações
incompletas sempre exigem intervenção.

### Alpha 5 — atualização pública conservadora

- [x] Expor plano de update somente leitura e fail-closed.
- [x] Persistir intenção de update vinculada ao plano e à evidência observada.
- [x] Exigir confirmação explícita e revalidar ownership imediatamente antes
  de qualquer mutação.
- [x] Executar substituição de um serviço preservando a unidade anterior (#54).
- [ ] Integrar verificação de saúde e decisão durável da Alpha 4.
- [ ] Expor rollback manual somente com evidência revalidada.

## Fase 3 — Ecossistema
- [ ] Adaptadores para outros runtimes.
- [ ] Integração opcional com monitoramento.
- [ ] Guias de migração e contribuições comunitárias.
