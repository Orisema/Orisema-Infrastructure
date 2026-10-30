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
- [x] Backup e restauração de configuração testados (#64).
- [x] Atualização controlada e rollback.
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
- [x] Conectar update à verificação e decisão durável após revalidação de
  ownership, journal de intenção e confirmação explícita (#56).
- [x] Conectar rollback manual somente após nova autorização e revalidação (#60).

Esta infraestrutura não executa retomada, update ou rollback. Operações
incompletas sempre exigem intervenção.

### Alpha 5 — atualização pública conservadora

- [x] Expor plano de update somente leitura e fail-closed.
- [x] Persistir intenção de update vinculada ao plano e à evidência observada.
- [x] Exigir confirmação explícita e revalidar ownership imediatamente antes
  de qualquer mutação.
- [x] Executar substituição de um serviço preservando a unidade anterior (#54).
- [x] Integrar verificação de saúde e decisão durável da Alpha 4 (#56).
- [x] Expor rollback manual somente com evidência revalidada (#60).

### Backup e restauração

- [x] Definir escopo, formato, integridade e modelo de segurança (#62).
- [x] Implementar backup journaled de configuração e histórico operacional (#64).
- [x] Implementar restore verificado somente em destino ausente (#64).
- [x] Validar corrupção, interrupção, concorrência e não sobrescrita em laboratório (#64).
- [ ] Definir contrato separado antes de qualquer backup de dados persistentes.

### Conectividade remota independente (decisão arquitetural)

- [ ] Implementar SSH remoto seguro com autenticação por chave, verificação estrita da identidade do host, menor privilégio e auditoria.
- [ ] Aceitar hosts alcançáveis por redes privadas/VPN já existentes, sem exigir contas ou serviços da comunidade Orisema.
- [ ] Suportar bastion SSH/ProxyJump configurado pelo operador, sem provisionar infraestrutura central.
- [ ] Validar inspeção remota somente leitura antes de permitir mutações, com testes para perda de conexão, CGNAT via rede previamente configurada e falha segura.
- [ ] Manter integrações com WireGuard, Tailscale, Headscale ou equivalentes estritamente opcionais e administradas pelo usuário.

**Princípio:** o Orisema Infrastructure funciona de maneira autônoma, sem dependência obrigatória de infraestrutura central, contas externas ou serviços hospedados pelo projeto. O Orisema não operará relay, VPN ou serviço de coordenação oficial. Atravessar NAT/CGNAT requer conectividade previamente estabelecida pelo operador (por VPN, proxy/bastion ou outro intermediário acessível); SSH isolado não garante esse acesso. Consulte [ADR 0002](docs/adr/0002-independent-connectivity.md).

### Transparência da cadeia de suprimentos

- [x] Adicionar geração de SBOM CycloneDX como artefato de CI.
- [x] Adicionar verificação informativa de vulnerabilidades Go.
- [ ] Publicar SBOM e checksum como ativos de cada release, com verificação automatizada.

## Fase 3 — Ecossistema
- [ ] Adaptadores para outros runtimes.
- [ ] Integração opcional com monitoramento.
- [ ] Guias de migração e contribuições comunitárias.

## Integrações opcionais — planejamento futuro

- [ ] Publicar RFC de contrato versionado para integrações, descoberta, autenticação, permissões e falha segura.
- [ ] Validar o contrato com serviço simulado antes de integrar outros projetos.
- [ ] Preservar a autonomia do Infrastructure: integrações não são requisito de instalação nem de release Alpha.

### Orisema Data (antigo RustDB)

- [ ] Avaliar o código atual do Orisema Data e definir contrato opcional de descoberta de instâncias e conexões.
- [ ] Iniciar por PostgreSQL com leitura e diagnóstico, sem exposição pública automática.
- [ ] Separar responsabilidades: Infrastructure gerencia o ciclo de vida de contêineres; Data gerencia conexões e lógica de banco.
- [ ] Exigir proteção de credenciais, menor privilégio, auditoria e comportamento fail-closed.
- [ ] Evoluir depois para outros bancos e editor SQL; backups somente após projeto e testes específicos.

**Fora do escopo inicial:** Gera Base, integração obrigatória, editor SQL completo e provisionamento irrestrito.
