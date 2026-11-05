# RFC-0006: distribuição assinada e instalação reversível

- Status: proposta para implementação incremental na Alpha 9
- Issues: #124, #135
- Artefatos: Linux AMD64 e ARM64
- Assinatura proposta: Sigstore keyless bundle
- Provenance proposta: GitHub artifact attestation

## Resumo

A Alpha 9 autenticará `checksums.txt` com uma assinatura Sigstore keyless e
publicará provenance para os arquivos da release. O instalador verificará
primeiro a identidade do workflow que assinou os checksums, depois a integridade
do arquivo da arquitetura selecionada, e somente então extrairá e executará o
binário. Uma atualização preservará o executável anterior até a validação final.

Checksum sem assinatura detecta corrupção, mas não prova quem publicou a lista.
Assinatura sem conferir identidade apenas prova que alguma credencial OIDC foi
usada. O contrato exige ambas as verificações e fixa repositório, workflow,
issuer e versão solicitada.

## Modelo de confiança

A raiz de confiança é composta por:

1. a trust root pública do Sigstore obtida e validada pelo Cosign;
2. o issuer OIDC `https://token.actions.githubusercontent.com`;
3. a identidade do workflow
   `https://github.com/Orisema/Orisema-Infrastructure/.github/workflows/release.yml@...`;
4. o repositório público `Orisema/Orisema-Infrastructure` usado na verificação
   de attestations;
5. a tag de release e o commit incorporado no binário.

O sufixo da identidade poderá ser `refs/heads/main` no fluxo gateado por issue
ou `refs/tags/<versão>` no fluxo acionado diretamente por tag. O verificador
não aceitará identidade de fork, outro workflow ou outro repositório. A política
exata usada por cada release será publicada na documentação e testada no próprio
workflow.

Sigstore keyless evita manter uma chave privada de longa duração no repositório.
Isso transfere confiança para OIDC, Fulcio, Rekor, trust-root do Sigstore e para
a segurança do workflow pinado. Comprometimento da conta/repositório ou dessas
dependências continua fora da capacidade de detecção local do checksum.

## Artefatos publicados

- `orisema_linux_amd64.tar.gz`;
- `orisema_linux_arm64.tar.gz`;
- `checksums.txt`;
- `checksums.txt.sigstore.json`;
- attestations armazenadas pelo GitHub para os três primeiros arquivos.

O bundle Sigstore contém assinatura, certificado e material de transparência;
não contém chave privada e não entra em `checksums.txt`, evitando uma dependência
circular.

## Ordem de produção

1. checkout do commit exato e validação da tag;
2. testes e build reproduzível;
3. verificação local dos checksums;
4. geração de provenance para archives e `checksums.txt`;
5. assinatura keyless de `checksums.txt`;
6. verificação imediata do bundle contra issuer e identidade esperados;
7. publicação de todos os assets;
8. download novo da release;
9. nova verificação de assinatura, checksums, attestations, versão e commit.

Falha em qualquer etapa impede a conclusão do workflow. Uma release parcial não
é considerada instalável e deve ser investigada; assets não são tratados como
confiáveis apenas porque a página da release existe.

## Verificação manual

O usuário baixa `checksums.txt`, seu bundle e o archive. Em ambiente online:

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp '^https://github.com/Orisema/Orisema-Infrastructure/.github/workflows/release.yml@refs/(heads/main|tags/v[0-9].*)$'
sha256sum --check checksums.txt
gh attestation verify orisema_linux_amd64.tar.gz \
  --repo Orisema/Orisema-Infrastructure
```

A expressão regular é limitada às duas origens de release contratadas. Antes da
Alpha 9 final, o workflow deverá testar e documentar a identidade efetivamente
emitida. Ampliações futuras exigem revisão deste contrato.

Verificação offline não faz parte do instalador inicial: ela exige distribuir e
atualizar trust material de forma separada. Falha de rede, Rekor, Fulcio ou
GitHub bloqueia a instalação segura em vez de fazer downgrade silencioso para
checksum não autenticado.

## Contrato do instalador

O instalador será um arquivo versionado e revisável, baixado antes de executar:

```sh
curl -fSLO https://raw.githubusercontent.com/Orisema/Orisema-Infrastructure/main/install.sh
less install.sh
sh install.sh --version v0.1.0-alpha.9 --user
```

Regras:

- Linux e arquitetura `x86_64`/`aarch64` são detectados explicitamente;
- `--version` é obrigatório na Alpha 9; `latest` não é inferido;
- downloads usam diretório temporário privado e redirects HTTPS com falha forte;
- assinatura vem antes do checksum, checksum antes da extração;
- archive deve conter somente o arquivo regular `orisema` esperado;
- versão e arquitetura reportadas pelo binário devem coincidir;
- destino padrão privilegiado é `/usr/local/bin`; `--user` usa
  `${XDG_BIN_HOME:-$HOME/.local/bin}` sem editar `PATH`;
- `sudo` é chamado apenas para a troca final no destino privilegiado;
- o binário anterior é preservado no mesmo filesystem durante a troca;
- falha na validação pós-instalação restaura o anterior;
- sem binário anterior, falha remove somente o arquivo novo criado por esta
  tentativa;
- desinstalação é um comando explícito e nunca remove containers ou estado.

## Downgrade, reinstalação e rollback

Reinstalar a mesma versão requer `--reinstall`. Instalar versão lexicalmente
anterior não é decidido por ordenação textual de prerelease: exige
`--allow-downgrade`. O rollback automático cobre apenas a CLI no destino; não
reverte manifestos, journals, containers ou dados.

O backup temporário do executável anterior é removido somente após `version`
confirmar a nova instalação. Interrupção entre rename e validação deve deixar
pelo menos um executável regular no destino, e o laboratório deve provar essa
propriedade.

## Rotação e resposta a incidente

Não há chave persistente para rotacionar. Mudança de nome do workflow, caminho,
repositório, issuer ou estratégia de ref altera a identidade e exige:

1. RFC ou adendo revisado;
2. atualização explícita do verificador e instalador;
3. release de transição quando possível;
4. registro no changelog e SECURITY.md.

Uma assinatura válida de identidade inesperada é rejeitada. Em incidente, a
release afetada deve ser marcada, o workflow interrompido e uma nova versão
publicada; assets de uma tag imutável não serão silenciosamente considerados
substitutos equivalentes.

## Implementação incremental

1. permissões OIDC/attestation mínimas e actions pinadas por SHA;
2. assinatura e auto-verificação de `checksums.txt`;
3. provenance e verificação após download;
4. script de verificação reutilizável;
5. instalador com destino de usuário;
6. troca privilegiada, upgrade reversível e desinstalação;
7. laboratórios Ubuntu/Fedora, adulteração, identidade e arquitetura negativas;
8. auditoria e gate da Alpha 9.
