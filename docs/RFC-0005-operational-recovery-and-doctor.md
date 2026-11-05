# RFC-0005: diagnóstico unificado e recuperação operacional

- Status: proposta para implementação incremental na Alpha 9
- Issues: #124, #125
- Contratos de manifesto: `orisema.dev/v1alpha1` a `orisema.dev/v1alpha4`
- Contrato operacional inicial: `orisema-doctor-v1`

## Resumo

A Alpha 9 reunirá diagnóstico de host, manifesto, bindings, runtime e journals
no comando somente leitura `orisema doctor`. O resultado será tipado, terá
próximo passo explícito e nunca será interpretado como autorização para mutar.

Operações incompletas continuarão preservando evidência. Uma mutação nova para
a mesma aplicação será bloqueada enquanto existir uma operação conflitante. A
recuperação executável será limitada a estados que tenham evidência íntegra e
uma ação já contratada; qualquer ambiguidade produzirá orientação manual e
falha fechada.

## Problema

O Orisema já possui diagnóstico de aplicação, `operation diagnose`, histórico
de recovery, reconciliação de `START` interrompido e rollback de update. Essas
interfaces são deliberadamente conservadoras, mas exigem que o operador saiba
qual journal consultar e qual comando especializado usar. Também não existe
ainda um gate comum aplicado a toda nova mutação para uma aplicação com operação
pendente.

Um reboot, encerramento do processo ou perda de SSH pode ocorrer entre a
persistência de uma etapa e a conclusão do comando externo. Nessa janela, o
estado atual do runtime não prova sozinho qual efeito ocorreu. Repetir a ação
automaticamente pode destruir a unidade anterior, exceder guardrails ou ocultar
evidência útil.

## Objetivos

- oferecer uma entrada única e somente leitura para diagnóstico pré-operacional;
- distinguir host, manifesto, dependências, runtime e journals;
- emitir checks tipados, severidade, evidência não sensível e próximo comando;
- detectar rootless/rootful, runtime indisponível e espaço insuficiente;
- detectar journal inseguro, corrompido, incompatível ou operação incompleta;
- impedir mutações conflitantes por aplicação antes da confirmação;
- reutilizar rollback e reconciliação existentes quando sua evidência estiver
  íntegra;
- manter valores de secrets fora de stdout, stderr, journal e fingerprints de
  diagnóstico.

## Fora do escopo

- retomada automática após reboot ou reconexão;
- inferir sucesso apenas porque um container está executando;
- apagar, reparar ou reescrever journal corrompido;
- rollback de banco, volume ou migração;
- matar processos concorrentes ou quebrar locks;
- recuperação distribuída ou mutação remota;
- transformar recomendação em autorização implícita.

## Interface proposta

```text
orisema doctor <manifest.yaml> --journal <directory>
orisema recover operation <manifest.yaml> --journal <directory> --operation <id>
```

`doctor` não solicita confirmação e não altera arquivos nem Podman. A saída
humana é o contrato inicial; uma saída estruturada somente será adicionada
depois que o schema for versionado separadamente.

Cada check possui:

- `id`: identificador ASCII estável;
- `scope`: `host`, `manifest`, `binding`, `runtime` ou `operation`;
- `status`: `PASS`, `WARN` ou `BLOCK`;
- `summary`: conclusão sem dados sensíveis;
- `next`: comando ou ação do operador, quando aplicável.

O exit code é `0` quando não existe `BLOCK`, `1` quando o diagnóstico foi
concluído com bloqueio ou ficou incompleto, e `2` para uso inválido da CLI.
`WARN` nunca libera uma mutação que outro check bloqueou.

## Checks mínimos de `doctor`

1. O manifesto é regular, não é symlink e passa pela validação existente.
2. Podman está disponível e informa um modo de execução reconhecido.
3. O diretório de journal é real, privado e não é symlink.
4. Todos os journals são regulares, privados, limitados e decodificáveis.
5. Não há operação incompleta conflitante para a aplicação.
6. Containers declarados têm ownership verificável ou são reportados como
   ausentes; colisões e drift bloqueiam.
7. Secrets e redes são resolvidos somente para identidade e revalidados sem
   imprimir valores ou locators sensíveis.
8. O filesystem do journal e o armazenamento do runtime têm margem mínima
   documentada; erro de medição bloqueia, pressão conhecida gera `BLOCK`.

Um journal corrompido não pode ser ignorado como se estivesse vazio. Um journal
de outra aplicação é reportado, mas não bloqueia a aplicação alvo salvo se a
evidência de escopo for ambígua.

## Gate comum de mutação

Antes de `deploy`, `start`, `stop`, `update apply`, `update rollback`, recovery
executável ou outra mutação local, o Orisema consulta o store operacional
configurado para aquela invocação. Se houver operação não terminal da mesma
aplicação, a nova mutação falha antes da confirmação e informa o ID, estado e o
comando `doctor` correspondente.

Como comandos legados ainda não recebem `--journal`, a migração será
incremental: primeiro os fluxos que já exigem journal; depois um diretório de
estado local explícito e documentado. Não haverá procura implícita em todo o
filesystem nem suposição silenciosa de um diretório global.

## Recuperação suportada

`recover operation` primeiro repete os checks relevantes de `doctor` e carrega
o journal pelo ID exato. Ação executável exige:

1. journal íntegro e versão suportada;
2. aplicação e serviço iguais ao manifesto fornecido;
3. lock exclusivo adquirido;
4. ownership, IDs e bindings revalidados;
5. estado com transição de recuperação já implementada;
6. confirmação que nomeia operação e ação;
7. evento de início sincronizado antes da mutação;
8. evento de resultado sincronizado depois dela.

Inicialmente, o comando poderá encaminhar apenas para o rollback de update em
`RECOVERY_REQUIRED` e para a reconciliação somente leitura de execução
interrompida. `PREPARING`, `APPLYING`, `VERIFYING`, `ROLLING_BACK`, operações de
arquivo e evidência incompleta permanecem manuais até existir prova específica
de retomada segura.

## Concorrência e interrupção

O lock de aplicação existente continua sendo a fronteira local de exclusão.
Diagnóstico pode executar concorrentemente porque é somente leitura, mas deve
reportar que a evidência pode mudar. Preparação e execução revalidam tudo sob o
lock apropriado. Encerramento depois do evento de início nunca causa retry
automático; a próxima execução volta ao diagnóstico.

## Segurança e privacidade

- nenhuma leitura de secret é necessária para checks de host e journal;
- resolução de binding produz apenas identidade e igualdade;
- mensagens não incluem valores, locators de arquivo nem ambiente público;
- caminhos exibidos são apenas os fornecidos explicitamente pelo operador;
- checks não seguem symlinks e rejeitam permissões amplas;
- `doctor` não corrige permissões silenciosamente;
- o resultado não é atestado nem substitui monitoramento externo.

## Implementação incremental

1. tipos e testes unitários dos checks;
2. `doctor` somente leitura para host, manifesto e journal;
3. runtime, bindings e espaço em disco;
4. gate comum nos fluxos mutáveis que já recebem journal;
5. `recover operation` sobre ações comprovadamente seguras;
6. laboratórios de kill, reboot, SSH e concorrência;
7. auditoria e documentação do gate da Alpha 9.

Distribuição assinada e instalação segura pertencem ao mesmo release, mas terão
contratos e PRs próprios para não acoplar a raiz de confiança ao executor de
recuperação.
