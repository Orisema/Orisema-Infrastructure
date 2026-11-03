# RFC-0003: configuração pública e redes Podman externas

- Status: proposta para Alpha 7
- Issue: #92
- Contrato candidato: `orisema.dev/v1alpha4`

## Resumo

A Alpha 7 adicionará configuração **explicitamente não secreta** por serviço e
associação a redes Podman externas preexistentes. O primeiro incremento não
cria, adota, altera nem remove redes. Toda rede é resolvida por identidade antes
do plano e essa identidade é revalidada imediatamente antes do deploy.

Secrets continuam exclusivamente no contrato da Alpha 6 e chegam como arquivos
em `/run/secrets`. Configuração pública poderá aparecer em `podman inspect`, no
ambiente do processo e para outros usuários com acesso ao runtime. Credenciais,
tokens, chaves e dados pessoais não podem usar este campo.

## Problema observado

O laboratório MixIdentity provou a entrega de secrets, mas o backend completo
ainda depende de parâmetros não secretos e comunicação privada entre
contêineres. O manifesto atual não representa essas propriedades; acrescentá-las
como argumentos ad hoc impediria que `plan` detectasse drift e confundiria
configuração pública com material secreto.

## Objetivos

- preservar `v1alpha1`, `v1alpha2` e `v1alpha3` sem mudança silenciosa;
- tornar inequívoco que a configuração declarada é pública;
- limitar nomes, valores, quantidade e tamanho total;
- resolver redes externas por nome exato e identidade imutável observada;
- comparar ambiente e associações de rede em `plan` sem imprimir valores;
- falhar antes de criar contêiner diante de ausência, ambiguidade ou troca de
  identidade;
- nunca remover ou reconfigurar uma rede externa.

## Fora do escopo inicial

- secrets em variáveis de ambiente;
- criação, remoção, IPAM, subnets, gateways ou drivers customizados;
- aliases DNS, IP fixo, publicação pública automática ou abertura de firewall;
- conexão ou desconexão de contêiner existente;
- update/rollback de aplicações que usem o novo contrato;
- redes remotas, multi-host, Kubernetes ou Docker;
- inferência de que um valor parece ou não uma credencial.

## Contrato candidato

```yaml
apiVersion: orisema.dev/v1alpha4
kind: Application
name: identity-lab

networks:
  - name: private
    source:
      podman:
        name: identity-lab-private

services:
  - name: api
    image: localhost/identity-api:lab
    publicEnvironment:
      - name: APP_MODE
        value: laboratory
      - name: HTTP_PORT
        value: "8080"
    networks:
      - name: private
    secrets:
      - name: database-password
        target: database-password
```

`publicEnvironment` é uma sequência, não um mapa, para rejeitar tipos YAML
implícitos e nomes duplicados de forma determinística. Cada entrada contém
exatamente `name` e `value`, ambos strings. Valores vazios são válidos.

`networks` no nível da aplicação declara referências. `networks` no serviço é
uma sequência de nomes lógicos declarados. Uma rede declarada e não consumida é
erro, evitando configuração morta e falsa expectativa de isolamento.

### Limites propostos

- no máximo 128 entradas de ambiente por serviço;
- nome conforme `[A-Za-z_][A-Za-z0-9_]{0,127}`;
- valor com até 4096 bytes, sem NUL, CR ou LF;
- soma de nomes e valores com até 32 KiB por serviço;
- no máximo 32 redes declaradas e 8 associações por serviço;
- nomes lógicos e nomes Podman seguem o identificador Orisema atual;
- cada serviço precisa de pelo menos uma rede quando o manifesto declara redes.

Qualquer presença de `publicEnvironment` ou `networks`, inclusive `null` ou
sequência vazia, é rejeitada por contratos anteriores com indicação explícita
de migração para `v1alpha4`.

## Resolução e identidade de rede

O provider inicial executa inspeção de metadados pelo nome Podman exato e
retorna somente:

- provider `podman`;
- nome lógico;
- locator operacional;
- ID imutável observado;
- driver necessário para validar compatibilidade local.

O resultado não carrega credenciais nem configuração IPAM completa. Nomes
vazios, IDs fora do formato esperado, mais de um resultado, rede ausente ou
driver diferente de `bridge` falham fechado no primeiro incremento.

O deploy revalida nome e ID imediatamente antes da criação e passa o ID ao
Podman, não apenas o nome mutável. O contêiner recebe um label por associação:

```text
dev.orisema.network.<logical>=podman:<immutable-id>
```

Esses labels pertencem ao contêiner, não à rede, e permitem comparar drift sem
adotar ownership sobre o recurso externo.

## Configuração pública no runtime

O deploy passa cada entrada ao Podman como argumento separado, sem shell. O
planner inspeciona o ambiente configurado e compara o mapa exato de nomes e
valores. Saídas humanas registram somente nomes e a frase “configuração pública
difere”; não imprimem valores, embora o contrato não prometa confidencialidade.

Variáveis adicionadas pela imagem precisam ser distinguidas das declaradas. O
contêiner recebe labels com os nomes gerenciados e um hash determinístico do
conjunto declarado; a comparação ainda lê o ambiente configurado para impedir
que um label adulterado prove equivalência.

Nenhum valor de `publicEnvironment` entra em journal ou labels. O backup de
configuração inclui o manifesto validado e, portanto, inclui esses valores
públicos por design. Isso reforça que o campo não oferece sigilo.

## Planejamento e mutação

1. `validate` verifica somente forma, referências e limites.
2. `plan` resolve metadados das redes e inspeciona contêineres existentes.
3. Rede ausente ou inválida bloqueia o plano inteiro antes de qualquer deploy.
4. `CREATE` exige a confirmação já existente.
5. Imediatamente antes de cada criação, todas as identidades de rede usadas
   pelo serviço são revalidadas.
6. O runtime cria o contêiner já associado às redes por ID.
7. Falha parcial usa o cleanup conservador existente apenas para o contêiner
   recém-criado com ownership exato; redes externas nunca são removidas.

Ambiente ou rede divergente em contêiner existente produz `DRIFT`. O primeiro
incremento não reconcilia nem substitui esse contêiner.

## Modelo de ameaça

| Risco | Controle inicial |
| --- | --- |
| Credencial declarada como pública | nome explícito, documentação e ausência de promessa de sigilo; secrets por arquivo continuam disponíveis |
| Troca de rede entre plano e deploy | identidade observada e revalidação imediata antes da mutação |
| Adoção ou remoção de rede alheia | provider externo somente leitura; nenhuma operação de lifecycle de rede |
| Associação à rede errada por nome reutilizado | deploy pelo ID revalidado e label de identidade no contêiner |
| Valor público aparecendo em saídas acidentais | erros e planos mencionam apenas nomes; valores não entram em labels ou journals; o backup do manifesto é exposição documentada |
| Argument injection | validação de nome e passagem por `exec` com argumentos separados, sem shell |
| Configuração fora do limite do host | limites por entrada, quantidade e total antes de consultar o runtime |
| Driver de rede fora do contrato testado | allowlist inicial somente para `bridge`, validada na resolução e revalidação |

## Compatibilidade e migração

`v1alpha4` é opt-in. Manifestos antigos continuam com a semântica publicada e
rejeitam os novos campos mesmo quando vazios. Um manifesto `v1alpha3` migra
alterando `apiVersion` somente depois que o operador classifica cada valor como
público e provisiona previamente as redes referenciadas.

## Sequência de implementação

1. implementar tipos e validação `v1alpha4`, incluindo presença de campos;
2. adicionar resolver de redes somente de metadados e testes negativos;
3. integrar resolução e comparação ao planner;
4. integrar ambiente e IDs de rede ao deploy com revalidação;
5. criar laboratório rootless de ausência, troca de identidade, drift e
   preservação da rede externa;
6. repetir o laboratório MixIdentity com backend/dependências artificiais e
   sem dados de produção;
7. reconciliar documentação e publicar somente após o gate completo.

Um probe prático em Podman 4.9.3 rootless confirmou IDs hexadecimais de 64
caracteres, driver `bridge`, criação de contêiner por ID e preservação da rede
depois da remoção do consumidor. Isso valida a premissa técnica, mas não
substitui os testes automatizados do provider e do planner.

## Gate de conclusão

- `go vet ./...`, `go test ./...` e `go test -race ./...` passam;
- `v1alpha1`–`v1alpha3` rejeitam toda presença dos novos campos;
- planos e erros não imprimem valores públicos, embora `inspect` possa expô-los;
- ausência, incompatibilidade e troca de identidade falham antes de mutação;
- drift de ambiente e rede é detectado sem substituição automática;
- laboratórios rootless provam que nenhuma rede externa é removida;
- o laboratório MixIdentity usa somente configuração e credenciais artificiais.

## Questões para incrementos posteriores

- lifecycle de redes gerenciadas exige journal e confirmação próprios;
- aliases DNS precisam de detecção de conflito no escopo da rede;
- arquivos de configuração não secretos podem merecer provider e diretório
  próprios, separados de `/run/secrets`;
- update/rollback precisam preservar e revalidar identidades de rede.
