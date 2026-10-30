# ADR 0003 — Fronteira segura de backup e restauração

- Status: Aceita para implementação
- Contexto: Fase 2 — Operação
- Issue: #62

## Contexto

O Orisema precisa produzir artefatos verificáveis e restaurá-los sem confundir
configuração, histórico operacional e dados persistentes. O manifesto atual
rejeita volumes, bind mounts, secrets e bancos de dados; portanto ainda não há
contrato capaz de assegurar ownership, quiescência, permissões ou consistência
de dados.

## Decisão

1. A primeira versão faz backup somente de um manifesto validado e, quando
   solicitado, dos journals locais da mesma aplicação. O conteúdo é copiado;
   caminhos de origem nunca são tratados como conteúdo restaurável.
2. Volumes, bind mounts, camadas de container, imagens, secrets, chaves,
   credenciais e dados de aplicações ficam fora do escopo.
3. O artefato é um diretório autônomo e versionado, publicado atomicamente com
   permissão `0700`. Arquivos internos usam `0600` e nomes fixos.
4. `metadata.json` usa um formato canônico, contém identidade aleatória do
   backup, aplicação, instante UTC, versão do formato, inventário ordenado,
   tamanho e SHA-256 de cada payload. O metadata também possui SHA-256 em um
   arquivo fixo separado. Integridade não significa autenticidade.
5. Backup recusa destino existente, symlink, arquivo especial, origem insegura,
   conteúdo mutável durante leitura e concorrência para a mesma aplicação.
6. Restauração sempre verifica estrutura, limites, hashes e conteúdo antes de
   escrever. Caminhos do artefato não controlam destinos no host.
7. O destino padrão deve estar ausente. Qualquer substituição futura exigirá
   modo explícito, confirmação vinculada a backup/aplicação/destino, snapshot
   anterior recuperável e journal; não pertence à primeira implementação.
8. Backup e restore são operações journaled. A intenção e o próximo passo são
   persistidos antes de criar, publicar ou materializar qualquer arquivo.
   Interrupções deixam staging identificável, nunca sucesso implícito.
9. Restore materializa em staging privado no mesmo filesystem, sincroniza,
   revalida que o destino continua ausente e usa rename atômico.
10. Journals restaurados são evidência histórica somente leitura e ficam em
    namespace de importação; não reativam nem retomam operações.

## Modelo de ameaça

- Artefato corrompido ou truncado: rejeitado antes de qualquer materialização.
- Path traversal, caminho absoluto, entrada duplicada ou symlink: rejeitados.
- Troca de origem/destino por symlink ou corrida: descritores e `lstat` são
  revalidados; falha fechada.
- Artefato muito grande ou com contagem excessiva: rejeitado por limites
  versionados antes da alocação/cópia.
- Destino criado entre plano e commit: restore aborta sem sobrescrever.
- Dois operadores: exclusão por aplicação no journal admite um vencedor.
- Operador malicioso com escrita no artefato: SHA-256 detecta corrupção
  acidental, mas não autentica; assinatura é evolução separada.

## Consequências

- O primeiro backup é útil para reconstruir configuração e preservar auditoria,
  mas não recupera estado de negócio.
- Não será anunciada consistência de banco de dados ou recuperação de volumes.
- Um contrato futuro de dados exige ADR próprio, ownership, driver por tipo de
  workload, quiescência/snapshot e laboratórios de restauração comprovados.
