# RFC-0008 — Substituição controlada de configuração declarativa

Status: **proposto para revisão humana; implementação runtime bloqueada**

Issues: [#164](https://github.com/Orisema/Orisema-Infrastructure/issues/164),
[#165](https://github.com/Orisema/Orisema-Infrastructure/issues/165)

Contratos existentes: `orisema.dev/v1alpha3`, `orisema.dev/v1alpha4` e
`orisema-update-intent-v2`

## Resumo

A Beta 1 propõe estender a substituição controlada da Alpha 8 para mudanças
declarativas de configuração pública, referências de secrets e associações a
redes externas. A operação continuará criando um candidato completo; nunca
alterará bindings do container em execução nem gerenciará o lifecycle dos
recursos referenciados.

Este documento não autoriza implementação. Parser, planner, journal e runtime
permanecem inalterados até a revisão humana do contrato e a divisão do trabalho
em incrementos verificáveis.

## Problema observado

O fluxo publicado aceita update apenas quando a diferença elegível é exclusiva
de imagem. Mudanças de configuração pública, secret ou rede produzem `DRIFT` e
bloqueiam a operação. Esse comportamento é seguro, mas obriga o operador a
recriar manualmente a aplicação e perde o caminho de preservação, verificação e
rollback já usado pelo update configurado.

Liberar todo `DRIFT` seria inseguro: diferenças desconhecidas poderiam entrar
no candidato, valores poderiam vazar na evidência e recursos externos poderiam
mudar entre planejamento e execução.

## Objetivos

- classificar diferenças por tipo e aceitar somente uma lista fechada;
- apresentar plano somente leitura sem transportar valores secretos;
- vincular confirmação a manifesto, alvo, diferenças e identidades observadas;
- criar um candidato com a configuração declarada completa;
- revalidar ownership, runtime e bindings antes de cada efeito dependente;
- preservar a unidade anterior até uma decisão durável;
- manter falhas e interrupções diagnosticáveis, sem cleanup destrutivo
  automático.

## Fora do escopo

- volumes, bancos, migrações ou rollback de dados;
- transação ou rollout coordenado entre serviços;
- criação, adoção, rotação ou remoção de secrets e redes externos;
- configuração secreta em variáveis de ambiente;
- mutação remota, Docker, multi-node ou recuperação automática;
- reconciliação de campos que o planner atual não compara.

## Invariantes

1. Plano e diagnóstico não autorizam mutação.
2. Nenhum efeito ocorre sem journal íntegro, admissão atômica, lock da
   aplicação e confirmação exata vinculada ao plano.
3. Valores de secrets nunca entram em stdout, stderr, plano, erro, journal,
   label, backup, teste gravado ou issue.
4. Valores de configuração pública continuam públicos no manifesto e runtime,
   mas o journal registra apenas nomes e fingerprints.
5. Locators privados de arquivo não são persistidos; o manifesto é fornecido e
   resolvido novamente em cada gate.
6. O conjunto de diferenças elegíveis é fechado e versionado. Diferença
   desconhecida, ambígua ou não comparada bloqueia a operação.
7. Recursos externos permanecem fora do lifecycle do Orisema.
8. A unidade anterior preservada é a única unidade de rollback; ela não é
   reconstruída usando o manifesto novo.

## Classificação proposta de diferenças

| Diferença | Beta 1 | Condição |
| --- | --- | --- |
| Imagem | elegível | contrato já publicado |
| Valor de configuração pública | candidata | nome e fingerprint tipados; valor nunca no journal |
| Inclusão/remoção de configuração pública | candidata | conjunto completo revalidado |
| Referência ou identidade de secret | candidata | provider, target e identidade válidos; sem valor ou locator persistido |
| Associação a rede externa | candidata | ID e driver revalidados; sem lifecycle |
| Porta, healthcheck ou restart policy | bloqueada inicialmente | exige decisão separada sobre impacto operacional |
| Volume, privilégio ou campo desconhecido | inelegível | fora do contrato suportado |
| Mais de um serviço com drift | inelegível inicialmente | nenhuma transação multi-serviço |

“Candidata” significa assunto sujeito à revisão desta RFC, não comportamento já
implementado. A implementação pode reduzir esse conjunto, mas não ampliá-lo
sem nova revisão.

## Plano e autorização

O planner deverá emitir diferenças tipadas em ordem canônica. Para secrets,
mostrará somente nome lógico, provider, target e mudança de identidade; para
configuração pública, mostrará nomes adicionados, removidos ou alterados, nunca
valores. A rede será representada por nome lógico, ID observado e driver.

O fingerprint de autorização deverá cobrir versão do contrato, aplicação,
serviço, container ID e ownership observados, imagem, diferenças tipadas,
configuração desejada e identidades externas. Qualquer mudança exige novo plano
e nova confirmação.

## Sequência mutável proposta

1. validar manifesto, resolver bindings e produzir o plano tipado;
2. persistir intenção sem valores e sem locators privados;
3. receber confirmação exata;
4. adquirir lock, reabrir o journal e reconstruir o plano;
5. admitir atomicamente a operação e persistir o próximo efeito;
6. preparar recursos transitórios já contratados, sem tocar a unidade atual;
7. revalidar bindings e preservar a unidade anterior;
8. criar o candidato completo usando argumentos separados, sem shell;
9. persistir a identidade do candidato e executar verify;
10. registrar decisão durável antes de habilitar conclusão ou rollback manual.

Depois que a unidade anterior for parada ou renomeada, nenhuma falha dispara
rollback ou remoção automática. O estado passa a exigir diagnóstico e ação
explícita conforme os contratos publicados.

## Falhas e recuperação

- identidade ou fingerprint trocado antes de um efeito: bloquear sem mutar;
- falha ao preparar binding transitório: preservar a unidade atual e registrar
  somente cleanup de ownership exato;
- falha após preservação da unidade anterior: manter candidato e evidência para
  diagnóstico, sem deduzir intenção;
- candidato unhealthy: registrar `RECOVERY_REQUIRED` e oferecer rollback
  manual revalidado;
- journal desconhecido, inseguro ou corrompido: bloquear todas as novas
  mutações sem apagar material;
- interrupção: a execução seguinte diagnostica; não repete efeitos
  automaticamente.

## Compatibilidade e downgrade

A intenção deverá ganhar versão nova somente se a forma persistida mudar.
Binários antigos devem rejeitar essa versão antes de qualquer efeito. O
instalador continuará bloqueando downgrade implícito; downgrade explícito não
torna uma operação Beta incompleta recuperável por uma versão anterior.

Manifestos `v1alpha3` e `v1alpha4` não ganham semântica nova por si só. A
capacidade pertence ao comando de substituição e à versão do journal, evitando
que um manifesto existente passe a autorizar mutação automaticamente.

## Plano de implementação após aprovação

1. tipos de diferença e plano somente leitura, com testes de redaction;
2. intenção versionada e confirmação vinculada, ainda sem mutação runtime;
3. admissão e criação do candidato para o menor subconjunto aprovado;
4. verify, decisão durável e rollback manual;
5. laboratórios de interrupção, concorrência, troca de bindings e saúde;
6. documentação, inventário da CLI, compatibilidade e gate de release.

Cada etapa terá issue e revisão próprias. Mudanças de concorrência ou
persistência exigirão `go test -race ./...`; efeitos Podman exigirão laboratório
isolado.

## Gate da RFC

- [ ] revisão humana das invariantes e da matriz de elegibilidade;
- [ ] decisão explícita sobre o menor subconjunto da primeira implementação;
- [ ] validação de que o plano não revela valores nem locators privados;
- [ ] compatibilidade e downgrade aceitos;
- [ ] cenários negativos e recuperação considerados suficientes;
- [ ] issues de implementação abertas somente após aprovação.
