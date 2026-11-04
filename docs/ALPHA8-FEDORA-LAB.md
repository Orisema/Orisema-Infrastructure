# Alpha 8 — laboratório isolado Fedora 43

Data: 04/11/2026.

Este registro descreve uma execução manual descartável do commit `f7a81b3` em
Fedora 43 AMD64 com Podman 5.8.9. O binário foi compilado estaticamente com Go
1.23.12 e teve SHA-256
`fd1398d3e0994a3ea85bf26085cbcf1b37e7eddb9a0e467c16d7b1324fc0fc08`.

## Ambiente e limite

O Fedora foi executado como contêiner privilegiado efêmero, com storage `vfs`,
sobre um host Ubuntu 24.04.5 com Podman rootless 4.9.3. O Podman dentro do
Fedora operou em modo rootful e isolado do storage e dos contêineres do host.

Esse resultado comprova compatibilidade funcional naquele userspace e versão do
Podman, mas não substitui um teste em host Fedora nativo, SELinux enforcing ou
Podman rootless Fedora. Não estabelece versão mínima suportada.

## Smoke test

O laboratório executou, em ordem:

1. `version` e `validate` com o binário Linux AMD64;
2. `plan` de uma aplicação Nginx com healthcheck e porta loopback;
3. `deploy` confirmado, seguido de `status` e requisição HTTP em
   `127.0.0.1:8081`;
4. inspeção de ownership, estado, healthcheck e imagem;
5. remoção do contêiner e comprovação de ausência.

Resultado: `FEDORA43_SMOKE_PASS`.

## Update configurado da Alpha 8

Um segundo laboratório criou uma aplicação `v1alpha4` com:

- Nginx `1.26-alpine` como unidade anterior;
- `APP_MODE=fedora-lab` como configuração pública;
- rede externa `bridge` preexistente e referenciada pelo manifesto;
- healthcheck `nginx -t`.

O manifesto alvo alterou somente a imagem para `1.27-alpine`. Os comandos
`update plan`, `update prepare`, `update authorize` e `update apply` passaram.
A inspeção final comprovou:

- candidato executando `docker.io/library/nginx:1.27-alpine`;
- `APP_MODE=fedora-lab` presente no candidato;
- candidato associado à rede `fedora-alpha8-private` pelo ID resolvido;
- unidade anterior preservada em estado `exited` com a imagem
  `docker.io/library/nginx:1.26-alpine`;
- `update verify` configurado recusado pelo bloqueio explícito esperado desta
  etapa da Alpha 8.

O candidato, a unidade anterior e a rede foram removidos e a ausência dos três
foi verificada antes de encerrar o contêiner Fedora.

Resultado: `FEDORA43_ALPHA8_APPLY_PASS`.

Secrets Podman e `file`, verify e rollback configurados não foram validados
neste laboratório específico e permanecem gates separados da Alpha 8.
