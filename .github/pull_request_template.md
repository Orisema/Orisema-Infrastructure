## Problema e escopo

<!-- Qual problema verificável esta mudança resolve? O que fica fora do escopo? -->

## Módulos e contratos afetados

<!-- Liste pacotes, comandos, formatos persistidos, manifestos e documentação. -->

- Módulos:
- Contratos públicos/persistidos:
- Compatibilidade e migração:

## Riscos, efeitos e recuperação

<!-- Inclua falha parcial, concorrência, interrupção e como recuperar sem apagar evidência. -->

- Efeitos no host/runtime:
- Dados ou evidências gravados:
- Pior falha plausível:
- Recuperação/rollback:

## Evidência de validação

<!-- Não marque um teste que não foi executado. Inclua comando e resultado observável. -->

- [ ] Caminho positivo exercitado
- [ ] Entradas inválidas e falhas relevantes exercitadas
- [ ] `go test ./...`
- [ ] `go vet ./...`
- [ ] Race detector quando há estado, concorrência ou persistência
- [ ] Laboratório Podman/SSH quando o runtime ou transporte muda
- [ ] Documentação e limitações atualizadas

Comandos e resultados:

```text

```

## Contrato de segurança

- [ ] A mudança continua fail-closed diante de identidade, ownership ou evidência ambígua
- [ ] Diagnóstico e plano não concedem autoridade de mutação
- [ ] Toda mutação suportada preserva confirmação, revalidação e journal durável
- [ ] Secrets, credenciais e conteúdo sensível não aparecem em logs, planos ou fixtures
- [ ] Resultados atribuídos a CI, host ou revisor foram realmente observados

## Revisão humana

<!-- Obrigatória para mudanças assistidas por IA e para fechar critérios que exigem mantenedor. -->

- [ ] Um mantenedor humano revisou problema, contratos, riscos e evidências
- Decisões ou ressalvas do mantenedor:

