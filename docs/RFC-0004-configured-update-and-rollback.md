# RFC-0004: update e rollback de aplicações configuradas

- Status: proposta para implementação incremental na Alpha 8
- Issue: #105
- Contratos de manifesto: `orisema.dev/v1alpha3` e `orisema.dev/v1alpha4`
- Contrato operacional candidato: `orisema-update-intent-v2`

## Resumo

A Alpha 8 estenderá a substituição controlada de um serviço para aplicações que
usam secrets externos, configuração pública e redes Podman externas. O update
continuará fail-closed: resolução, identidade, ownership, intenção persistida e
confirmação serão revalidados antes da primeira mutação e novamente nos limites
em que uma dependência externa possa mudar.

O journal nunca armazenará valores de secrets ou de configuração pública. Ele
persistirá somente a versão do contrato, fingerprints determinísticos e
metadados de identidade necessários para provar que `prepare`, `authorize`,
`apply` e `verify` se referem ao mesmo alvo. Recursos externos permanecem fora
do lifecycle do Orisema.

## Problema observado

As Alphas 6 e 7 permitem criar aplicações reais com secrets por arquivo,
configuração pública e redes externas. O fluxo de update/rollback da Alpha 5,
porém, bloqueia qualquer serviço com esses bindings e cria candidatos apenas com
imagem, portas, healthcheck e restart policy. Remover somente esse bloqueio
criaria um candidato incompleto e poderia conectá-lo a uma rede ou secret cuja
identidade mudou depois do plano.

O laboratório MixIdentity saudável deve permanecer intacto enquanto este
contrato é implementado e testado em alvos descartáveis.

## Objetivos

- atualizar um serviço configurado por substituição controlada, nunca por
  mutação in-place;
- reutilizar os resolvers e a entrega segura das Alphas 6 e 7;
- vincular todas as fases à mesma configuração desejada sem persistir valores;
- revalidar identidade de secrets e redes antes de qualquer mutação dependente;
- preservar a unidade anterior como caminho explícito de rollback;
- deixar falhas parciais diagnosticáveis, sem rollback ou cleanup automáticos
  que destruam evidência;
- manter compatibilidade com updates antigos sem bindings.

## Fora do escopo

- transação de update entre vários serviços;
- rollback de banco de dados, volumes ou outros dados persistentes;
- criação, adoção, alteração ou remoção de secrets e redes externos;
- aliases, IP fixo, IPAM ou lifecycle de rede;
- mutação remota, Docker, multi-host ou Kubernetes;
- secrets em variáveis de ambiente;
- retomada automática de uma operação interrompida.

## Invariantes

1. Nenhuma mutação ocorre sem journal durável, confirmação exata, lock da
   aplicação e revalidação do container ID e ownership observados.
2. Valores de secrets nunca atravessam plano, resultado, erro, log, label,
   journal ou backup operacional.
3. Valores de `publicEnvironment` não entram no journal nem em labels. Eles
   continuam públicos no manifesto, backup de configuração e runtime.
4. Um locator de arquivo pode revelar informação operacional e também não é
   persistido na intenção. A resolução parte novamente do manifesto fornecido.
5. O candidato recebe o conjunto declarado completo de portas, healthcheck,
   restart policy, configuração pública, secrets e redes; não existe aplicação
   parcial de bindings.
6. O Orisema nunca muta recursos externos referenciados. Secrets Podman
   transitórios usados pelo provider `file` são a única exceção já contratada e
   têm ownership e cleanup exatos.
7. O container anterior preservado é a unidade de rollback. O rollback não o
   recria a partir do manifesto atual.

## Evidência persistida

`UpdateIntent` evolui para `orisema-update-intent-v2` e inclui:

- fingerprint do plano e dos campos operacionais já cobertos na Alpha 5;
- fingerprint canônico da configuração desejada;
- versão do contrato de bindings;
- identidades não secretas resolvidas de secrets e redes;
- nomes gerenciados de configuração pública e seu digest, sem valores;
- container ID anterior, imagem atual e imagem alvo.

O fingerprint da configuração é calculado em memória sobre a representação
canônica validada. Ele prova igualdade, não permite reconstruir a configuração.
Para secrets, a evidência inclui provider, nome lógico, target e identidade
imutável observada, mas não valor nem locator. Para redes, inclui provider, nome
lógico, ID imutável e driver validado.

Alterar manifesto, configuração pública, identidade de secret ou identidade de
rede depois de `prepare` invalida a intenção. O operador deve gerar e autorizar
uma nova intenção.

## Planejamento e elegibilidade

O planner resolve todos os bindings antes de classificar o serviço. Falha de
resolução bloqueia o plano inteiro e não produz candidato parcial.

O primeiro incremento remove o bloqueio para update de imagem quando os
bindings declarados permanecem equivalentes e podem ser resolvidos. Em seguida,
o mesmo contrato poderá classificar rotação declarada de configuração ou
identidade como substituição controlada. Cada expansão exige diferenças
tipadas; texto livre de drift nunca torna um serviço elegível.

Um candidato exige:

- exatamente um serviço e um container gerenciado com ownership válido;
- healthcheck declarado;
- diferenças limitadas ao conjunto explicitamente suportado;
- configuração e bindings integralmente resolvidos;
- nenhuma ambiguidade, drift desconhecido ou recurso externo incompatível.

## Sequência da operação

### Prepare

1. validar o manifesto e resolver secrets e redes;
2. inspecionar o runtime e construir plano tipado;
3. persistir `UpdateIntent` v2 com fingerprints e identidades;
4. não criar secret transitório, parar ou renomear containers.

### Authorize

1. adquirir o lock da aplicação;
2. reconstruir o plano e resolver bindings novamente;
3. exigir igualdade exata com a intenção persistida;
4. registrar autorização sem realizar mutação no runtime.

### Apply

1. exigir a frase de confirmação exata;
2. reconstruir e comparar plano, ownership e fingerprints;
3. persistir o limite de execução antes da primeira mutação;
4. preparar bindings transitórios em etapa journaled própria;
5. revalidar redes e secrets imediatamente antes de criar o candidato;
6. parar e renomear a unidade anterior conforme a máquina de estados existente;
7. criar o candidato com o conjunto declarado completo, usando argumentos
   separados e sem shell;
8. persistir container ID e evidência do candidato.

A ordem final entre preparação transitória e preservação do container anterior
deve garantir duas propriedades: qualquer criação de recurso transitório já
possui journal durável e uma falha anterior à parada deixa o serviço atual
intacto. O código compartilhará o construtor de argumentos de deploy para
evitar semânticas divergentes entre CREATE e update.

### Verify

O verify inspeciona o container candidato por ID exato, resolve novamente os
bindings desejados e compara a evidência com a intenção. Saúde aprovada só
produz `COMPLETE` quando ownership, configuração e identidades permanecem
válidos. Divergência ou falha de saúde produz `RECOVERY_REQUIRED`.

### Rollback

O rollback revalida journal, decisão, ownership e IDs antes de qualquer
mutação. A unidade anterior preservada conserva sua configuração, mounts e
associações originais; ela é restaurada sem reconstrução a partir do manifesto
atual e sem alterar recursos externos.

Se um secret ou uma rede externa necessários à unidade anterior tiver sido
removido pelo operador, a tentativa de start pode falhar. O Orisema registra a
falha e mantém `RECOVERY_REQUIRED`; não cria substitutos nem adota recursos com
o mesmo nome. O candidato com falha permanece disponível para diagnóstico até
uma ação explícita posterior.

## Falhas e recuperação

- identidade trocada em qualquer gate bloqueia a próxima mutação;
- falha ao preparar um secret transitório antes da parada preserva a unidade
  atual e remove apenas o recurso transitório de ownership exato;
- após a unidade anterior ser parada ou renomeada, falha não provoca rollback
  automático e exige diagnóstico e rollback explícito;
- interrupções são retomadas somente como diagnóstico; a CLI não deduz intenção
  nem repete efeitos automaticamente;
- adulteração ou versão desconhecida de journal falha fechado;
- recursos externos nunca entram no conjunto genérico de recursos removíveis
  da operação.

## Sequência de implementação

1. introduzir intenção v2, representação canônica e fingerprints sem valores;
2. tornar plan/prepare/authorize conscientes de bindings, ainda sem mutação;
3. criar candidato com a entrega resolvida completa e limite journaled para
   recursos transitórios;
4. integrar verify e rollback à nova evidência;
5. cobrir falhas, interrupções, concorrência e compatibilidade em testes;
6. validar Podman rootless com secrets `podman` e `file`, configuração pública e
   rede externa descartável;
7. repetir update, falha e rollback do MixIdentity em ambiente artificial
   isolado, sem tocar a instância saudável existente;
8. reconciliar documentação, auditoria e release somente após o gate completo.

A etapa 1 e as fases somente leitura da etapa 2 foram implementadas nas issues
#107 e #109. `apply`, `verify` e rollback para serviços configurados permanecem
bloqueados até que a entrega completa e seus limites journaled sejam
implementados.

A criação do candidato com bindings completos e a fronteira journaled de
preparação transitória foram implementadas na issue #111. Verify e rollback
configurados agora resolvem novamente os bindings, exigem o fingerprint
autorizado e inspecionam a configuração completa antes de registrar saúde ou
iniciar qualquer mutação de rollback.

Os gates finais exercitam concorrência nos limites de preparação e rollback,
falha fechada quando uma identidade muda entre fases, Podman rootless nativo e
a topologia MixIdentity artificial isolada. No MixIdentity, uma falha forçada
do candidato web produziu `RECOVERY_REQUIRED`; o rollback restaurou o mesmo ID
da unidade anterior, passou o healthcheck e persistiu `ROLLED_BACK`, mantendo
bancos, Keycloak e rede externa fora do lifecycle do Orisema.

## Gate de conclusão

- `go vet ./...`, `go test ./...` e `go test -race ./...` passam;
- updates antigos sem bindings conservam o comportamento publicado;
- nenhum canário secreto aparece em stdout, stderr, journals, labels, backups
  operacionais ou artefatos de teste;
- mudança de secret, arquivo, rede ou configuração entre fases falha fechado;
- ausência, recriação ou troca de identidade de rede falha antes da mutação
  dependente;
- falhas de candidato e interrupções preservam evidência e oferecem rollback
  manual revalidado;
- cleanup remove somente secrets transitórios e containers de ownership exato;
- laboratórios rootless comprovam update saudável e rollback após falha;
- o MixIdentity artificial passa pelo fluxo completo em ambiente isolado.

## Questões para incrementos posteriores

- política de retenção e remoção explícita do container anterior;
- arquivos públicos de configuração com provider próprio;
- update coordenado entre serviços dependentes;
- integração com volumes após contrato específico de dados e consistência.
