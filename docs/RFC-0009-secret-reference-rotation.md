# RFC-0009 — Rotação de referência de secret por substituição

Status: **proposto para revisão humana; implementação runtime bloqueada**

Issues: [#183](https://github.com/Orisema/Orisema-Infrastructure/issues/183),
[#184](https://github.com/Orisema/Orisema-Infrastructure/issues/184)

Contratos existentes: `orisema.dev/v1alpha3`,
`orisema-update-intent-v2` e `orisema-update-intent-v3-public-config-prepare`

## Resumo

A Beta 2 propõe permitir a rotação declarativa de uma referência de secret por
substituição controlada de um único container. O Orisema nunca altera o secret
externo, lê seu valor para planejamento ou atualiza o container em execução.
Ele resolve metadados, preserva a unidade anterior, cria um candidato completo,
verifica saúde e oferece rollback manual.

Este documento não autoriza implementação. Parser, planner, journal e runtime
permanecem inalterados até a revisão humana deste contrato e a aprovação
explícita do menor subconjunto.

## Problema

Hoje um update de imagem preserva e revalida secrets já declarados, mas uma
troca de referência, identidade ou target produz `DRIFT` inelegível. Para girar
uma credencial, o operador precisa recriar a aplicação fora do fluxo durável e
perde a unidade de rollback.

Liberar todo drift de secret seria inseguro. Uma diferença textual não prova
qual mount mudou; um locator pode revelar informação operacional; um provider
pode trocar a identidade entre fases; e o provider `file` exige staging
transitório por descritor com riscos de TOCTOU e cleanup próprios.

## Objetivos

- classificar diferenças de secret por tipo fechado e ordem canônica;
- planejar usando somente provider, nome lógico, target e identidades;
- manter valores e locators privados fora de toda evidência persistida e saída;
- vincular intenção, confirmação e admissão ao mesmo container e diferença;
- revalidar a identidade desejada imediatamente antes do efeito dependente;
- criar o candidato com a configuração declarada completa;
- preservar a unidade anterior como único rollback;
- manter os secrets externos fora do lifecycle da operação.

## Menor subconjunto recomendado

O primeiro incremento deve aceitar exatamente uma diferença em exatamente um
serviço:

- um binding já existente mantém nome lógico e `target`;
- o provider antigo e o novo são `podman`;
- a referência declarada pode mudar e deve resolver para uma nova identidade
  Podman não vazia;
- o secret desejado já existe e foi criado fora desta operação;
- imagem, configuração pública, redes, portas, healthcheck, restart policy e
  todos os demais bindings permanecem idênticos;
- o serviço possui healthcheck declarado e container gerenciado inequívoco.

Isso cobre a rotação prática: criar previamente um novo secret Podman e mudar a
declaração para ele. Não exige remover o secret antigo enquanto a unidade
anterior ainda o consome.

## Matriz de elegibilidade

| Diferença | Primeiro incremento | Motivo |
| --- | --- | --- |
| Referência Podman de um binding existente | elegível | nova identidade externa previamente resolvida |
| Identidade diferente sob a referência declarada | elegível | detecta recriação/troca e exige novo plano |
| Mudança de nome lógico ou `target` | bloqueada | altera contrato de consumo da aplicação |
| Inclusão ou remoção de binding | bloqueada | amplia o conjunto de mounts |
| Provider `file` | bloqueado | exige contrato separado de staging, locator e cleanup |
| Troca `file` ↔ `podman` | bloqueada | muda fronteira de confiança e entrega |
| Mais de um secret | bloqueada | reduz ambiguidade e superfície do primeiro gate |
| Secret combinado com outro drift | bloqueado | nenhuma composição implícita de autorizações |
| Valor inline ou secret em ambiente | inelegível | schema e política não transportam valores |
| Mais de um serviço | inelegível | não há transação multi-serviço |

Qualquer diferença desconhecida ou texto livre continua sendo bloqueio. A
implementação pode reduzir esta tabela, mas não ampliá-la sem nova revisão.

## Invariantes

1. Plano e diagnóstico não autorizam mutação.
2. Nenhuma mutação ocorre sem store íntegro, lock da aplicação, intenção
   durável, confirmação exata e revalidação fresca.
3. Valores de secrets não entram em stdout, stderr, logs, erros, planos,
   confirmações, journals, labels, backups, testes gravados ou issues.
4. Locators privados não são persistidos. O manifesto é fornecido e resolvido
   novamente em cada gate.
5. A intenção persiste somente metadados allowlisted e identidades externas;
   nenhuma identidade é derivada do conteúdo do secret.
6. A operação nunca cria, adota, atualiza ou remove o secret referenciado.
7. O container anterior preservado é o rollback e nunca é reconstruído a
   partir do manifesto novo.
8. Depois da primeira mutação, nenhuma falha dispara rollback ou cleanup
   automático.

## Plano e diferença tipada

O plano somente leitura deve mostrar que uma rotação é candidata sem mostrar o
locator antigo ou novo. A saída permitida contém aplicação, serviço, nome
lógico, target, provider e fingerprints/identidades abreviadas apenas se essa
forma já estiver documentada como não sensível. A forma inicial preferida é
simplesmente `SECRET_REFERENCE_ROTATION name target provider=podman`.

O fingerprint de diferença cobre versão, aplicação, serviço, container ID,
ownership, nome lógico, target, provider, identidade anterior e identidade
desejada. O fingerprint de configuração cobre o conjunto completo de bindings
e campos suportados. Alterar qualquer elemento exige novo plano.

## Intenção e confirmação

Uma forma persistida nova é necessária para que binários antigos rejeitem a
operação antes de qualquer efeito. O identificador candidato é
`orisema-update-intent-v4-secret-rotation`.

A intenção registra:

- fingerprints do plano, diferença e configuração completa;
- provider, nome lógico, target e identidades antiga/nova;
- container ID e ownership observados;
- imagem inalterada e timestamp do plano.

Ela não registra locator, valor, conteúdo, argumento de runtime ou material de
staging.

As confirmações candidatas são:

```text
AUTHORIZE SECRET ROTATION application/service operation-id
APPLY SECRET ROTATION application/service operation-id
```

Authorize reabre o store, resolve o manifesto e compara o plano completo sem
mutar Podman. Apply repete a revalidação sob admissão atômica antes de persistir
o primeiro efeito.

## Sequência mutável

1. persistir `APPLYING` e o próximo efeito antes da mutação;
2. revalidar container atual, referência desejada e identidade Podman;
3. parar e renomear a unidade anterior para o slot único de rollback;
4. resolver novamente a mesma identidade desejada;
5. criar o candidato completo passando o ID do secret ao runtime por argumento
   separado, sem shell e sem conteúdo;
6. persistir o ID retornado do candidato;
7. executar verify explicitamente e registrar decisão durável;
8. habilitar rollback manual somente após `RECOVERY_REQUIRED` íntegro.

Não há etapa de secret transitório no subconjunto Podman. O secret anterior
permanece externo e consumido pela unidade preservada; o desejado permanece
externo e consumido pelo candidato.

## Verify e rollback

Verify revalida o fingerprint desejado, a identidade do candidato, a unidade
anterior e a identidade externa do novo secret. Saúde terminal positiva decide
`COMPLETED`; não sucesso terminal decide `RECOVERY_REQUIRED`; evidência
inconclusiva espera sem polling ou mutação.

Rollback exige confirmação já publicada, revalida os containers exatos e o
manifesto desejado, preserva o candidato falho em slot diagnóstico e restaura
o mesmo ID anterior. Ele não precisa resolver ou reconstruir a referência
antiga: o container preservado já contém seu mount. `rollback-verify` exige
saúde positiva antes de `ROLLED_BACK`.

## Modelo de ameaça

| Ameaça | Controle exigido |
| --- | --- |
| Valor atravessa observabilidade | estruturas metadata-only e canários buscados em toda evidência |
| Locator aparece no journal/erro | allowlist persistida sem locator; erros identificam nome lógico |
| Secret desejado é recriado entre fases | identidade faz parte do fingerprint e é revalidada antes da criação |
| Manifesto troca outro binding | fingerprint da configuração completa bloqueia |
| Container atual é substituído | ID e ownership exatos bloqueiam antes do efeito |
| Nome aponta para recurso alheio | resolução não concede ownership nem lifecycle; operador autoriza identidade observada |
| Candidato recebe secret diferente | criação usa a identidade revalidada, nunca o locator mutável |
| Concorrência inicia duas rotações | admissão atômica e lock por aplicação permitem um vencedor |
| Interrupção após preservar anterior | pending step e IDs permanecem; somente diagnóstico/rollback explícito |
| Remoção do secret antigo | rollback usa o container preservado; falha de start permanece diagnosticável |
| Digest do conteúdo permite dicionário | nenhum conteúdo ou hash derivado do valor é calculado |

## Provider `file` adiado

O provider `file` continuará suportado para deploy e update de imagem, mas não
para rotação neste primeiro incremento. Uma expansão futura precisa decidir:

- como representar mudança sem persistir o path;
- como manter o descritor validado até o handoff;
- quando criar e remover o secret Podman transitório;
- como provar ownership e ausência de consumidores no cleanup;
- como recuperar interrupções em cada fronteira sem reabrir o arquivo.

Misturar essas decisões com o gate Podman reduziria a capacidade de atribuir
falhas e ampliaria desnecessariamente a fronteira de confiança.

## Compatibilidade e downgrade

Manifestos `v1alpha3`/`v1alpha4` não ganham autorização automática; a
capacidade pertence ao comando e à intenção v4. Binários anteriores rejeitam a
versão desconhecida antes de qualquer efeito. Downgrade explícito da CLI não
torna uma intenção v4 incompleta recuperável por uma versão anterior.

Updates de imagem v2 e substituições públicas v3 mantêm seus contratos. Não há
migração automática de journal nem conversão entre intenções.

## Plano de implementação após aprovação

1. diferença tipada e plano somente leitura com testes de redaction;
2. intenção v4 e autorização, ainda sem runtime;
3. apply Podman com revalidação e candidato completo;
4. verify e rollback manual;
5. concorrência, interrupção, troca de identidade e laboratórios com canários;
6. documentação, compatibilidade e gate de release.

Cada etapa terá issue e revisão próprias. Persistência/concorrência exige race
detector; efeitos runtime exigem Podman isolado. Nenhuma issue de implementação
runtime deve ser aberta antes da aprovação humana desta RFC.

## Gate da RFC

- [ ] invariantes e matriz revisadas por humano;
- [ ] subconjunto Podman de um binding aprovado explicitamente;
- [ ] plano e journal sem valor ou locator aceitos;
- [ ] estados, confirmação, compatibilidade e downgrade aceitos;
- [ ] cenários negativos e recuperação considerados suficientes;
- [ ] implementação autorizada somente em issues incrementais posteriores.

## Referências

- [RFC-0002 — secrets seguros](RFC-0002-secure-secrets.md)
- [RFC-0004 — update configurado](RFC-0004-configured-update-and-rollback.md)
- [RFC-0008 — substituição declarativa](RFC-0008-controlled-configuration-replacement.md)
- [Contrato v1alpha3](MANIFEST-V1ALPHA3.md)
- [Inventário da CLI](CLI-INVENTORY.md)
