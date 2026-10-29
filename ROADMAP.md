# Roadmap

Etapas propostas, sem datas prometidas.

O plano verificável para as versões experimentais está em
[docs/ALPHA_ROADMAP.md](docs/ALPHA_ROADMAP.md). As Issues #12 e #13 concluíram
versionamento/distribuição e documentação/compatibilidade; regressão e segurança
do release permanecem na Issue #14.

## Fase 0 — Fundação
- [x] Criar organização e repositório públicos.
- [x] Publicar manifesto e documentação inicial.
- [x] Escolher licença open source (Apache-2.0).
- [ ] Definir mantenedores e decisões arquiteturais.

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
- [ ] Conectar update/rollback somente após revalidação de ownership, journal
  de intenção e confirmação explícita.

O primeiro slice não executa retomada, update ou rollback. Operações
incompletas sempre exigem intervenção.

## Fase 3 — Ecossistema
- [ ] Adaptadores para outros runtimes.
- [ ] Integração opcional com monitoramento.
- [ ] Guias de migração e contribuições comunitárias.
