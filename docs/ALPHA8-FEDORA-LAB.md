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

## VM Fedora 45 nativa

Um segundo gate, mais representativo, foi executado em 04/11/2026 numa VM
Fedora 45 AMD64 nativa:

- usuário `jgomes`, UID 1000, sem elevação de privilégio;
- SELinux `Enforcing`;
- Podman 6.1.3 rootless;
- storage `overlay` e runtime `crun`;
- binário com o mesmo SHA-256 registrado acima.

O laboratório usou nomes `fedora-native-a8` exclusivos e porta loopback 18081.
O manifesto `v1alpha4` combinou Nginx, configuração pública, rede externa
`bridge`, healthcheck e um secret artificial pelo provider `file`, armazenado
num diretório privado 0700 com arquivo 0600.

Foram comprovados:

1. `validate`, plano `CREATE`, deploy rootless e resposta HTTP;
2. leitura interna do secret montado, comparada sem imprimir seu conteúdo;
3. classificação `UPDATE_CANDIDATE` para `1.26-alpine` → `1.27-alpine`;
4. `prepare`, `authorize` e `apply` da intenção v2;
5. candidato com imagem nova, configuração pública, secret e rede preservados;
6. unidade anterior em `exited` com a imagem antiga;
7. ausência de secrets `orisema-transient-*` após a criação;
8. rejeição esperada de `update verify` configurado nesta etapa;
9. remoção do candidato, unidade anterior, rede e diretório temporário;
10. auditoria remota final confirmando a ausência de todos os recursos acima.

Resultado: `FEDORA45_NATIVE_ALPHA8_PASS` e
`FEDORA45_NATIVE_CLEANUP_VERIFIED`.

O material secreto usado foi exclusivamente artificial. Nenhum projeto ou
contêiner preexistente da VM fez parte do laboratório. Verify e rollback
configurados continuam como próximos gates funcionais da Alpha 8.
