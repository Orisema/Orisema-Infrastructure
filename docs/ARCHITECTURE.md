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
- **Build info:** metadados imutáveis incorporados ao binário durante o build,
  com valores explícitos de desenvolvimento quando não injetados.

## Separação de responsabilidades atual

- **Planner:** compara somente os campos declarativos suportados e produz
  `CREATE`, `NOOP`, `DRIFT` ou `REVIEW`.
- **Diagnostic:** combina o resultado declarativo com execução e saúde
  observadas para recomendar uma ação somente informativa.
- **Recovery plan:** transforma o diagnóstico em ações tipadas, pré-condições e
  riscos, sem possuir capacidade de mutação do runtime.
- **Runtime:** lista e inspeciona contêineres e executa operações explicitamente
  solicitadas pela CLI.

Saúde não altera o plano declarativo. Diagnóstico não executa recuperação. Essa
separação permite que um contêiner parado, mas equivalente nos campos
declarativos, continue `NOOP` enquanto o diagnóstico informa a falha operacional.
Um plano de recuperação pode recomendar `START`, mas não executa a recomendação.

## Fora do escopo inicial

Orquestração multi-node, painel web e gestão central de identidade.

## Distribuição experimental

O pipeline acionado somente por tags compila binários Linux AMD64 e ARM64 com
`CGO_ENABLED=0`, `-trimpath` e metadados via linker. Os arquivos compactados e
checksums são verificados antes da criação da GitHub Release. O comando
`orisema version` apenas lê dados incorporados e informações do runtime Go; não
acessa a rede nem arquivos externos.

## Decisões atuais e abertas

Go 1.23 é a linguagem e versão mínima declarada pelo módulo. Podman em Linux é
o primeiro runtime; Docker, Windows e macOS não são suportados. O manifesto usa
`orisema.dev/v1alpha1`, ainda experimental. Modelo de estado futuro, gestão de
segredos, volumes e estratégia de migração do manifesto permanecem decisões
abertas. A matriz entre artefatos produzidos e ambientes realmente executados
está em [COMPATIBILITY.md](COMPATIBILITY.md).
