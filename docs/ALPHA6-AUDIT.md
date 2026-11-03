# Auditoria de conclusão da Alpha 6

Esta auditoria fecha o escopo de segredos seguros definido pela
[RFC-0002](RFC-0002-secure-secrets.md). Ela não declara suporte de produção nem
inclui configuração comum, redes, rotação automática ou mutação remota.

## Resultado do gate

| Propriedade | Evidência | Resultado |
| --- | --- | --- |
| Contratos antigos preservados | testes de manifesto rejeitam `secrets`, inclusive vazio ou nulo, em `v1alpha1`/`v1alpha2` | Passou |
| Valor fora do modelo | provider API, planner, runtime e lifecycle transportam somente referência, identidade e metadados | Passou |
| Entrega Podman | `alpha6-podman-secrets-lab.sh` monta pelo ID imutável em `/run/secrets`, prova `NOOP` e detecta rotação como `DRIFT` | Passou |
| Ausência e indisponibilidade | referência removida falha antes de mutação no laboratório Podman | Passou |
| Provider `file` | testes rejeitam permissões inseguras, symlink, hardlink, arquivo especial, tamanho excedido e troca de identidade | Passou |
| Handoff e cleanup | laboratório por arquivo prova reinício do consumidor e ausência de secrets transitórios após a criação | Passou |
| Ciclo de vida | laboratório prova create/list/inspect, bloqueio por consumidor, confirmação exata e preservação de recurso externo | Passou |
| Não divulgação | cada laboratório busca negativamente canários em todas as evidências persistidas | Passou |
| Aplicação real | `alpha6-mixidentity-secrets-lab.sh` usa a imagem local `mix-identity-web:1.0.0`, credencial artificial e recursos exclusivos | Passou localmente em Podman 4.9.3 rootless |

O laboratório MixIdentity verifica a fronteira de infraestrutura: a imagem real
recebe o arquivo artificial com modo somente leitura e o plano converge para
`NOOP`. Ele não afirma que o backend completo do produto foi configurado nem
usa dependências ou dados de produção. O contêiner de laboratório preexistente
não é adotado, alterado ou removido.

## Matriz de verificação final

Antes da publicação, o commit candidato precisa passar:

```sh
go vet ./...
go test ./...
go test -race ./...
scripts/alpha6-podman-secrets-lab.sh
scripts/alpha6-file-secrets-lab.sh
scripts/alpha6-secret-lifecycle-lab.sh
scripts/alpha6-mixidentity-secrets-lab.sh
```

Os três primeiros laboratórios executam na matriz automatizada. O laboratório
MixIdentity é local porque sua imagem pertence ao ambiente do operador e não é
publicada nem enviada ao CI. O workflow da tag repete vet, testes, race detector,
build AMD64/ARM64, checksums e instalação do artefato AMD64 publicado.

## Limites preservados

- O Orisema nunca imprime, exporta, registra em journal ou inclui em backup o
  conteúdo de um segredo.
- Mudança de identidade é drift; não há rotação automática nem substituição
  silenciosa de consumidores.
- Update e rollback de aplicações com bindings de segredos continuam
  bloqueados até existir operação autorizada específica.
- Configuração não secreta e redes permanecem no roadmap como contrato futuro
  separado e não fazem parte da Alpha 6.

## Release verificada

A pré-release [`v0.1.0-alpha.6`](https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.6)
foi publicada em 3 de novembro de 2026 no commit
`92a352ccbdb241ff2e371839d780a52ad8951598`. O workflow de release terminou com
sucesso e a verificação independente após a publicação confirmou:

- `2125c470f4032c239bfa52b4466838fbee19e71208d1dc25d9037bd24c0bd285`
  para `orisema_linux_amd64.tar.gz`;
- `5973043f71e9661365b22eb2a4076dc633a8f27480feaffbef021d637d9b550f`
  para `orisema_linux_arm64.tar.gz`;
- o binário AMD64 baixado reportou `Version: v0.1.0-alpha.6`, o commit completo
  acima, Go 1.23.12 e alvo `linux/amd64`.
