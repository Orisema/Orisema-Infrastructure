# Auditoria de conclusão da Alpha 8

Data: 04/11/2026. Escopo: update e rollback de aplicações configuradas conforme
a [RFC-0004](RFC-0004-configured-update-and-rollback.md).

Esta auditoria não declara prontidão para produção. O rollback continua sendo
de container de aplicação, sem garantia de reversão de dados, volumes ou
migrações.

## Resultado do gate

| Propriedade | Evidência | Resultado |
| --- | --- | --- |
| Intenção durável | contrato v2, evidência canônica e fingerprint sem valores | Passou |
| Autorização | confirmação exata, TTL e revalidação antes da mutação | Passou |
| Candidato configurado | ambiente público, secrets e redes preservados | Passou |
| Verify | bindings resolvidos novamente e configuração integral inspecionada | Passou |
| Falha fechada | rotação de identidade bloqueia sem modificar o journal | Passou |
| Rollback | mesmo ID anterior restaurado sem reconstrução | Passou |
| Recursos externos | redes, providers e dependências fora do lifecycle | Passou |
| Concorrência | um vencedor em preparação de bindings e início de rollback | Passou |
| Não divulgação | nenhum canário em journal, saída ou evidência | Passou |
| Compatibilidade | updates antigos sem bindings mantêm o fluxo publicado | Passou |

## Ambientes exercitados

- Ubuntu 24.04 AMD64 e Podman 4.9.3 rootless no laboratório local/CI;
- Fedora 45 AMD64 nativo, Podman 6.1.3 rootless, SELinux `Enforcing`, storage
  `overlay` e runtime `crun`;
- Fedora 43 AMD64 descartável, Podman 5.8.9 rootful com `vfs`;
- MixIdentity artificial isolado com PostgreSQL, Keycloak, PHP-FPM e nginx.

No Fedora 45, o fluxo configurado combinou secret pelo provider `file`,
configuração pública, rede externa e healthcheck. A falha forçada do candidato
produziu `RECOVERY_REQUIRED`; o rollback restaurou a unidade anterior, passou o
healthcheck, persistiu `ROLLED_BACK` e deixou o host sem recursos do laboratório.

No MixIdentity, a imagem web real foi atualizada entre referências locais
exclusivas do laboratório. Bancos, Keycloak e rede externa permaneceram fora
da operação. O candidato foi parado deliberadamente, o mesmo container anterior
foi restaurado e todos os recursos e tags temporários foram removidos.

## Matriz de verificação da release

O commit candidato deve passar:

```sh
go vet ./...
go test ./...
go test -race ./...
scripts/build-release.sh
scripts/verify-release.sh
```

A CI acrescenta CodeQL, quickstarts Ubuntu e laboratórios Podman. O workflow da
tag reconstrói os arquivos Linux AMD64/ARM64, verifica os checksums, publica a
pré-release, baixa novamente os artefatos e executa o binário AMD64 confirmando
versão e commit incorporados.

A revisão estática local também procurou chaves privadas e formatos comuns de
credenciais incorporadas, sem encontrar candidato. Chamadas de subprocesso,
arquivos privados, journals, transporte SSH e fronteiras de remoção foram
revisadas contra os controles existentes. O scanner durável Codex Security não
estava disponível na sessão de preparação; portanto esta auditoria não afirma
ter produzido um relatório desse scanner e se apoia em revisão local, CodeQL e
testes source-backed.

## Limites preservados

- sem rollback de dados, volumes ou migrações;
- sem retomada automática após interrupção;
- sem lifecycle de redes externas ou providers de secrets;
- sem aliases DNS gerenciados, secrets por ambiente ou configuração de
  permissões avançadas;
- sem update coordenado entre múltiplos serviços;
- sem assinatura de artefatos ou provenance attestation;
- Docker, macOS e Windows continuam não suportados.

## Publicação verificada

A issue [#121](https://github.com/Orisema/Orisema-Infrastructure/issues/121)
foi o gate explícito da `v0.1.0-alpha.8`. Ela foi encerrada pelo
[PR #122](https://github.com/Orisema/Orisema-Infrastructure/pull/122) após a CI
verde. O workflow de release publicou a tag imutável a partir do commit
`c12ebb2bbdb90c6bad7e80a8c0ac439238f91848`, baixou os artefatos publicados e
executou novamente o binário AMD64.

A verificação independente posterior confirmou ambos os arquivos com
`sha256sum --check` e confirmou no binário AMD64 a versão `v0.1.0-alpha.8` e o
commit completo acima. Checksums publicados:

```text
cc48562479e73e587051b7f0c905b74ca936c52937b0584fbc255ade5590c268  orisema_linux_amd64.tar.gz
d6fec50737ad38e4517a02d293a432e2d23464fcaf56d9afee9bc50f77bc72b8  orisema_linux_arm64.tar.gz
```

Release: [v0.1.0-alpha.8](https://github.com/Orisema/Orisema-Infrastructure/releases/tag/v0.1.0-alpha.8).
