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

## Fora do escopo inicial

Orquestração multi-node, painel web e gestão central de identidade.

## Decisões abertas

Linguagem (Go é uma candidata), modelo de estado, formato do manifesto, gestão de segredos e estratégia de testes. Não presumir suporte a Windows/macOS até que seja implementado e testado.
