# Auditoria da Alpha 7 — configuração pública e redes externas

Data da execução local: 03/11/2026. Ambiente: Linux, Podman 4.9.3 rootless.

## Resultado

| Gate | Evidência | Resultado |
| --- | --- | --- |
| Contratos antigos | `v1alpha1`–`v1alpha3` rejeitam os novos campos | Passou |
| Configuração pública | CREATE entrega por argumentos separados; plano detecta drift sem imprimir valores | Passou |
| Rede externa | resolução e revalidação por ID, driver `bridge`, conexão por ID | Passou |
| Lifecycle | remoção dos containers gerenciados preserva redes e dependências externas | Passou |
| Aplicação real | app e nginx reais do MixIdentity com PostgreSQL e Keycloak artificiais isolados | Passou |
| Repetibilidade | laboratório exclusivo recusa adoção e limpa somente nomes próprios | Passou |

O script `scripts/alpha7-config-network-lab.sh` cobre CREATE, NOOP, drift de
ambiente/rede e preservação de redes. O script
`scripts/alpha7-mixidentity-lab.sh` acrescenta a topologia real: dois bancos
PostgreSQL, Keycloak, backend PHP-FPM e nginx. As dependências usam nomes,
credenciais e dados exclusivamente artificiais.

O laboratório completo confirmou:

- dependências artificiais saudáveis antes do deploy;
- backend criado pelo Orisema e conectado à rede externa declarada;
- nginx real alcançando `GET /health/live` no backend real;
- plano combinado com `NOOP` para app e web;
- ausência dos canários artificiais nas evidências capturadas;
- rede e dependências externas presentes depois da remoção dos containers
  gerenciados.

## Fronteiras preservadas

As imagens MixIdentity pertencem ao ambiente do operador e não são publicadas
por este repositório. Quando elas não existem, o CI registra um skip explícito;
os testes genéricos de Alpha 7 continuam obrigatórios e usam imagens públicas.

O nginx MixIdentity possui o upstream `identity-app` compilado. Aliases DNS não
fazem parte do contrato Alpha 7 porque ainda precisam de detecção de conflito.
Por isso o laboratório aplica esse alias somente ao container isolado, entre o
deploy do app e o deploy do nginx. O Orisema continua sem criar, alterar ou
remover aliases ou redes externas.

Algumas credenciais artificiais precisam ser fornecidas como ambiente porque a
aplicação real ainda não implementa equivalentes `_FILE`. Elas são declaradas
como configuração explicitamente pública apenas neste laboratório descartável;
isso não autoriza credenciais reais em `publicEnvironment`. Nenhum valor é
copiado para labels, planos ou artefatos de evidência.

## Conclusão

O gate funcional da Alpha 7 está concluído para CREATE local em Podman rootless.
Update/rollback, lifecycle de rede, aliases DNS e credenciais de aplicações que
aceitam somente ambiente permanecem fora do contrato e devem continuar
fail-closed.
