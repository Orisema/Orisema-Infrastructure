# RFC-0001 — Standalone, Managed Agent e gerenciamento de serviços

- **Status:** Proposta para discussão
- **Data:** 2026-10-29
- **Escopo:** Orisema Infrastructure
- **Implementação:** Nenhuma alteração de comportamento nesta RFC

## Resumo

Manter a CLI Orisema utilizável sem daemon (**Standalone**) e propor um agente opcional (**Managed**) para supervisão e reconciliação controlada. O Orisema deve integrar-se aos gerenciadores nativos do sistema operacional, sem substituí-los.

## Motivação e estado atual

A implementação experimental atual prioriza Linux e Podman. `plan` é somente leitura, `deploy` exige confirmação, e `NOOP` não implica processo saudável ou em execução. O projeto ainda não garante recuperação automática após reboot nem rollback autônomo. A Alpha 4 exige revalidação de ownership, journal e confirmação explícita antes de operações mutáveis.

Esta RFC não altera esses contratos.

## Objetivos

1. CLI independente de agente e de serviços em segundo plano.
2. Gerenciamento explícito do ciclo de vida de serviços nativos.
3. Agente opcional, supervisionado pelo próprio SO, com instalação e desinstalação reversíveis.
4. Observação de saúde, estado desejado e reconciliação auditável.
5. Adaptadores por plataforma, com matriz de capacidades e degradação explícita.
6. Segurança por padrão, mínimo privilégio e ausência de adoção implícita de recursos.

## Não objetivos iniciais

- Alta disponibilidade distribuída, cluster ou painel central.
- Suporte de produção a Windows, macOS, Docker ou sistemas legados.
- Substituir systemd, SCM, launchd ou políticas do runtime.
- Reiniciar, reparar, atualizar ou fazer rollback sem consentimento/política explícita.
- Tratar disponibilidade de rede como garantia de acesso a dependências remotas.

## Modos de operação

### Standalone

A CLI executa comandos sob demanda e compartilha bibliotecas de manifesto, planejamento, runtime, inspeção e diagnóstico. Nenhum daemon é exigido. Comandos existentes preservam suas semânticas.

### Managed (opt-in)

`orisema-agent` é processo opcional supervisionado pelo gerenciador nativo. Lê somente recursos declarados e explicitamente autorizados. Primeiro marco: observação e relatório sem mutações. Futuramente, reconciliação de estado com política explícita e limites.

A CLI pode consultar o agente, mas deve funcionar quando ele estiver ausente.

## Componentes propostos

- `internal/services`: interface de capacidades, listagem, status, enable/disable, start/stop, sempre com autorização.
- `internal/services/systemd`: primeiro adaptador experimental Linux; Windows SCM e macOS launchd somente após testes específicos.
- `internal/agent`: loop de observação, cancelamento, limites e shutdown.
- `internal/health`: sondas com timeout, limites de concorrência e histórico.
- `internal/reconcile`: compara estado desejado e observado; emite propostas, sem executar por padrão.
- `cmd/orisema-agent`: binário separado opcional, se aprovado.

A divisão exata em pacotes é ilustrativa e será validada contra o código existente.

## Contratos de segurança

- Instalação do agente requer ação explícita e não deve ocorrer implicitamente por `deploy`.
- Preferir execução não privilegiada; operações administrativas exigem autorização isolada.
- Não adotar serviços ou contêineres preexistentes sem verificação de ownership e consentimento.
- Configuração local com permissões restritas; evitar segredos em logs.
- Comunicação CLI/agente local, autenticada e autorizada (ex.: socket Unix com credenciais do peer; alternativa Windows a definir).
- Registrar decisões e ações com logs estruturados, sem expor credenciais.
- Reinicializações com backoff, limite e proteção contra loops; healthcheck falho não implica reinício automático.
- Preservar journal durável e confirmação explícita para operações destrutivas e alterações de configuração.
- Não presumir equivalência de capacidades entre sistemas operacionais.

## Fluxo de reconciliação proposto

1. Ler manifesto versionado e validar esquema.
2. Identificar runtime, SO e capacidades.
3. Observar estado atual e evidências de ownership.
4. Produzir plano somente leitura com motivo e impacto.
5. Se houver política aprovada, revalidar imediatamente antes da ação.
6. Executar apenas ações permitidas, com journal e limites.
7. Verificar resultado, registrar falhas e exigir intervenção quando houver incerteza.

**Padrão inicial:** `observe-only`. Qualquer modo de correção automática é assunto de RFC/implementação posterior.

## Manifesto ilustrativo — não implementado

```yaml
apiVersion: orisema.dev/v1alpha-proposal
kind: ManagedService
metadata:
  name: exemplo
spec:
  runtime: podman
  startup:
    enabled: true
  management:
    mode: observe-only
  health:
    type: http
    endpoint: /health
    interval: 30s
    timeout: 5s
```

Não interpretar esse exemplo como suporte atual nem introduzi-lo no parser antes de aprovar um contrato versionado.

## Compatibilidade e suporte

| Ambiente | Situação atual | Proposta |
| --- | --- | --- |
| Linux + Podman | Experimental suportado | Primeiro alvo para protótipo de serviço e agente |
| Linux + systemd | Não há contrato de agente | Adaptador inicial |
| Linux legado (SysVinit/OpenRC) | Sem suporte oficial | Investigar por capacidades |
| Windows SCM | Sem suporte oficial | Futuro adaptador |
| macOS launchd | Sem suporte oficial | Futuro adaptador |
| Docker | Sem suporte oficial | Futuro adaptador de runtime |

## Etapas e critérios de aceite

**Etapa A — desenho e testes de contrato:** interfaces, matriz de capacidades, modelos de ameaça, testes unitários; nenhum comportamento alterado.

**Etapa B — serviços nativos Linux:** listar/inspecionar unidades permitidas e gerar plano; mutações somente com confirmação; testes em VM descartável.

**Etapa C — agente observador:** instalação/desinstalação explícita, inicialização via systemd, desligamento limpo, logs, métricas básicas, CLI independente; sem recuperação automática.

**Etapa D — reconciliação limitada:** política opt-in, revalidação de ownership, journal, limites de tentativas, idempotência, testes de reboot, falha de rede, processo travado e queda do agente.

**Etapa E — portabilidade:** protótipos Windows/macOS e Linux legado, cada um com testes de integração e documentação das capacidades ausentes.

A aprovação de cada etapa exige testes automatizados, documentação de riscos e manutenção dos contratos atuais de `plan`, `deploy`, `status` e recuperação Alpha 4.

## Questões abertas

1. O agente deve usar binário separado ou subcomando interno?
2. Qual a fronteira entre políticas de restart do Podman e reconciliação do agente?
3. Quais ações são seguras para autorização antecipada?
4. Qual formato de estado local e política de retenção de eventos?
5. Quais versões mínimas de systemd e quais ambientes legados serão testados?
6. Como impedir competição entre agentes e instâncias da CLI no mesmo host?

## Referências internas

- [Roadmap](../ROADMAP.md)
- [Planejamento atual](PLANNING.md)
- [Compatibilidade](COMPATIBILITY.md)
- [Recuperação Alpha 4](ALPHA4-RECOVERY-DESIGN.md)
