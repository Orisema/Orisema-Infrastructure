# RFC-0002 — Segredos por arquivo e provedores seguros

- **Status:** Proposta para implementação incremental
- **Data:** 2026-11-03
- **Escopo:** Alpha 6 do Orisema Infrastructure
- **Issue:** #77
- **Implementação:** Nenhuma alteração de comportamento nesta RFC

## Resumo

Introduzir um contrato versionado para referenciar segredos externos e
entregá-los a contêineres como arquivos somente leitura em
`/run/secrets/<target>`. O primeiro incremento terá o provedor `podman` como
caminho recomendado e o provedor `file` como compatibilidade local. O Orisema
nunca aceitará o valor de um segredo no manifesto nem o projetará em variável
de ambiente.

Esta RFC define a fronteira de segurança antes de alterar o parser, o planner ou
o runtime. A sintaxe mostrada aqui é ilustrativa e ainda não é aceita pela CLI.

## Motivação

Os laboratórios com aplicações reais mostraram que imagens úteis dependem de
credenciais e configuração externas. O contrato `orisema.dev/v1alpha2` rejeita
corretamente esses campos, mas isso impede implantar aplicações que precisam de
senhas, tokens ou chaves sem recorrer a comandos Podman fora do Orisema.

Colocar valores em YAML, argumentos, variáveis de ambiente ou arquivos
versionados resolveria a ergonomia à custa de exposição em Git, histórico do
shell, `inspect`, logs, planos, journals e backups. A Alpha 6 deve permitir o
caso de uso sem criar esses canais.

Configuração não secreta, redes customizadas, volumes e descoberta de serviços
continuam sendo decisões separadas. Esta RFC não tenta tornar uma aplicação
real completa apenas por adicionar segredos.

## Objetivos

1. Referenciar segredos sem incluir seus valores no manifesto.
2. Entregar cada segredo como arquivo somente leitura sob `/run/secrets`.
3. Tornar `podman` o provedor inicial recomendado e `file` uma opção explícita
   de compatibilidade.
4. Preservar `validate` e `plan` como operações sem divulgação de valores.
5. Falhar antes de mutar quando uma referência estiver ausente, insegura,
   ambígua ou tiver mudado durante a autorização.
6. Definir ciclo de vida, limites, ownership, redaction e comportamento de
   rotação antes da implementação.
7. Manter operação rootless e independência de serviços externos.

## Não objetivos do primeiro incremento

- Segredos inline, `.env`, environment variables ou expansão de templates.
- Exibir, exportar, recuperar ou incluir segredos em backup.
- Criar um cofre, KMS, serviço central ou protocolo de sincronização.
- Distribuir segredos para hosts remotos ou permitir mutação remota.
- Rotação automática, hot reload ou reinício automático de consumidores.
- Ocultar do operador autorizado o nome de uma referência ou o caminho de um
  arquivo local; esses metadados não são valores secretos.
- Proteger contra `root`, comprometimento da conta Unix que executa o Orisema ou
  comprometimento do runtime. Esses atores já podem observar o processo ou os
  arquivos acessíveis à conta.

## Contrato proposto

Uma próxima versão de manifesto, provisoriamente `orisema.dev/v1alpha3`, deverá
adicionar declarações de referências no nível da aplicação e ligações por
serviço. `v1alpha1` e `v1alpha2` continuarão rejeitando esses campos.

```yaml
apiVersion: orisema.dev/v1alpha3
kind: Application
name: exemplo

secrets:
  - name: database-password
    source:
      podman:
        name: exemplo-database-password
  - name: legacy-api-key
    source:
      file:
        path: /home/operator/.config/orisema/secrets/legacy-api-key

services:
  - name: app
    image: registry.example/app:1.0.0
    secrets:
      - name: database-password
        target: database-password
      - name: legacy-api-key
        target: legacy-api-key
```

O exemplo não autoriza implementação antes dos testes de contrato. Exatamente
um tipo de `source` será obrigatório. `name`, `target` e referências terão
formato estrito, tamanho limitado e unicidade no escopo correspondente.
`target` será apenas um nome de arquivo, nunca caminho, e produzirá exatamente
`/run/secrets/<target>`; valores como `..`, separadores, nomes absolutos e
duplicatas serão rejeitados.

O limite inicial proposto é 64 segredos por aplicação e 64 KiB por segredo.
Esses limites devem ser constantes testadas e documentadas na implementação,
não alocações controladas livremente pelo manifesto.

## Provedores

### `podman` — recomendado

Referencia um segredo que já existe no armazenamento da instância Podman do
usuário atual. O Orisema resolve a referência, verifica metadados e instrui o
runtime a montá-la no alvo declarado.

- O manifesto contém nome/referência, nunca conteúdo.
- O Orisema não usa nem oferece equivalente a `podman secret inspect
  --showsecret`.
- IDs, nomes, driver e labels estritamente necessários podem aparecer em saída
  de inspeção; o conteúdo não.
- Um segredo preexistente não é adotado ou removido implicitamente.
- A CLI de ciclo de vida só gerencia segredos com ownership inequívoco do
  Orisema e recusa remoção enquanto houver consumidores conhecidos.

### `file` — compatibilidade explícita

Referencia um arquivo absoluto local administrado pelo operador. É útil para
integração com provisionadores existentes, mas expõe o caminho de origem nos
metadados locais/runtime e oferece menos isolamento de ciclo de vida que o
provedor `podman`.

Antes do uso, a implementação deverá verificar, sem seguir links, que:

- o arquivo é regular, pertence ao usuário efetivo, possui no máximo `0600` e
  tem exatamente um hard link;
- nenhum componente resolvido é symlink e os diretórios pais não permitem
  escrita a outros usuários;
- tamanho e tipo respeitam os limites antes e depois da leitura;
- a mesma identidade aberta e validada é entregue ao backend, sem reabrir um
  caminho vulnerável a troca.

A técnica de handoff por descritor ou staging privado precisa ser comprovada em
laboratório com a versão mínima suportada do Podman. Um bind que reabra o
caminho sem proteção contra corrida não passa o gate.

## Entrega ao contêiner

O primeiro contrato aceita somente `type=mount`, com arquivo somente leitura em
`/run/secrets/<target>`. Não haverá opção de destino arbitrário nem projeção em
ambiente. A implementação deve verificar permissões efetivas dentro de um
contêiner de laboratório e documentar diferenças impostas pelo Podman.

O conteúdo não entra no modelo do planner. Planos comparam a identidade da
referência, o provedor, o alvo e a ligação ao serviço. Eles não calculam nem
persistem hash simples do valor: um digest reutilizável permitiria ataques de
dicionário contra segredos de baixa entropia.

Mudança do valor é uma rotação explícita. Ela exige nova operação autorizada e
substituição controlada dos consumidores; `NOOP` não promete que o conteúdo
externo permaneceu igual. Imediatamente antes de qualquer mutação, o executor
revalida a identidade da referência e, para `file`, mantém a identidade já
aberta durante o handoff.

## CLI de ciclo de vida

O primeiro desenho para segredos gerenciados pelo Orisema é:

```text
orisema secret create NAME --provider podman --stdin
orisema secret list
orisema secret inspect NAME
orisema secret remove NAME
```

`create` não aceitará `--value`; leitura interativa de terminal será recusada
até existir prompt seguro testado. `list` e `inspect` mostram somente metadados
allowlisted. Não existe comando `show`, `export` ou flag de revelação. `remove`
exige ownership, verificação de consumidores, confirmação vinculada à
identidade e revalidação imediatamente antes da remoção.

O provedor `file` é administrado externamente: a CLI não sobrescreve nem remove
seu arquivo de origem.

## Modelo de ameaça e controles

| Ameaça | Controle exigido |
| --- | --- |
| Valor commitado no manifesto | Schema não possui campo para conteúdo; campos desconhecidos falham |
| Vazamento em saída, erro ou journal | Estruturas de resultado contêm apenas metadados allowlisted; testes com canários |
| Vazamento por argumento/process list | Sem `--value`; conteúdo somente por stdin/descritor seguro |
| Vazamento por ambiente ou `inspect` | Sem projeção em ambiente; wrapper nunca pede conteúdo ao runtime |
| Path traversal ou alvo sobreposto | `target` é basename estrito, único e preso a `/run/secrets` |
| Symlink, hardlink ou troca TOCTOU | Resolução sem links, `nlink == 1`, validação por descritor e handoff da mesma identidade |
| Arquivo permissivo ou de outro usuário | Ownership do EUID e modo máximo `0600`; falha fechada |
| Referência trocada após o plano | Revalidação sob exclusão imediatamente antes da mutação |
| Recurso Podman alheio adotado/removido | Labels/ownership exatos e nenhuma adoção implícita |
| Exaustão por tamanho ou contagem | Limites versionados antes de leitura, alocação ou criação |
| Segredo capturado por backup | Diretórios, providers e conteúdo permanecem explicitamente excluídos |
| Contêiner não consumidor acessa segredo | Montagem somente nos serviços declarados, um alvo por ligação |

Um manifesto não confiável ainda pode escolher qual segredo nomeado entregar a
um serviço. Portanto aplicar um manifesto requer que o operador revise as
referências no plano e autorize a operação. O Orisema não transforma posse do
manifesto em autorização irrestrita sobre todos os segredos do usuário.

## Redaction e persistência

Valores de segredo são dados proibidos em:

- stdout, stderr, logs e mensagens de erro;
- planos, confirmações, journals, snapshots e diagnósticos;
- labels e argumentos do contêiner;
- backups, relatórios de suporte e testes gravados.

Testes usarão canários únicos e buscarão o valor integral e representações
codificadas nas saídas e artefatos. Redaction por substituição textual é defesa
secundária; a defesa primária é não transportar o valor por estruturas de
observabilidade.

Backups podem preservar o manifesto com referências, pois ele já é
configuração declarativa, mas nunca resolvem nem copiam o conteúdo. Restore não
cria segredos: referências ausentes bloqueiam o próximo plano/deploy.

## Falhas e disponibilidade

`validate` verifica forma e regras estáticas sem resolver conteúdo. `plan`
resolve apenas capacidades e metadados necessários, sem revelar o valor. Uma
referência ausente, provider indisponível, permissão insegura, runtime
incompatível ou mudança de identidade produz falha fechada antes de criar,
substituir ou remover contêiner.

Falha parcial na preparação não pode deixar segredo temporário órfão sem
identidade e journal suficientes para diagnóstico e limpeza explícita. O
Orisema não apaga automaticamente um recurso quando não consegue provar que o
criou e que nenhum consumidor o usa.

## Rootless, remoto e portabilidade

O alvo inicial é Podman rootless no mesmo host e na mesma conta Unix que
executa a CLI. Segredos do Podman de outra conta ou de root não são
implicitamente acessíveis. Docker, Windows e macOS permanecem sem suporte.

Operações remotas continuam somente leitura no contrato atual. O registry SSH
não armazenará referências de segredos e o transporte remoto não enviará
conteúdo. Suporte remoto futuro exigirá autenticação, journal e resolução no
host de destino em RFC própria.

## Sequência de implementação e gates

1. **Contrato:** aprovar esta RFC, casos negativos e sintaxe final de
   `v1alpha3`, preservando `v1alpha1`/`v1alpha2`.
2. **Provider API:** implementar interface interna e fake provider; provar que
   planner/resultados não carregam valores.
3. **Podman:** resolver/montar segredo preexistente e testar permissões,
   indisponibilidade, ownership, canários e versões suportadas.
4. **File:** implementar resolução/handoff resistente a corrida e laboratórios
   de symlink, hardlink, troca concorrente e permissões.
5. **Lifecycle:** adicionar comandos create/list/inspect/remove sem exposição,
   com confirmação e consumer tracking.
6. **Aplicação real:** repetir o laboratório MixIdentity com credenciais
   artificiais e dependências isoladas, sem usar dados de produção.

Nenhuma etapa será anunciada como concluída apenas porque o caso feliz passou.
O gate exige `go test`, race detector, vet, testes de integração Podman e busca
negativa dos canários em todas as evidências geradas.

## Consequências e questões abertas

- Aplicações que só aceitam credenciais por ambiente precisarão ser adaptadas
  para ler arquivos; suporte por ambiente não entra silenciosamente depois.
- O caminho do provider `file` é metadado observável e deve ser tratado como
  informação operacional sensível, embora não seja o valor.
- A implementação deverá decidir a estratégia exata de labels/consumer
  tracking após testar capacidades nas versões mínimas do Podman.
- Configuração não secreta e redes são os próximos bloqueios observados no
  laboratório e precisam de contratos separados.

## Referências internas

- [Contrato atual do manifesto](MANIFEST-V1ALPHA2.md)
- [Roadmap Alpha](ALPHA_ROADMAP.md)
- [Arquitetura](ARCHITECTURE.md)
- [Fronteira segura de backup](adr/0003-safe-backup-boundary.md)
- [Operação com Podman](PODMAN.md)
