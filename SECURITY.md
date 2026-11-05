# Segurança

**Não existe versão pronta para produção.**

Não publique credenciais, tokens, dados pessoais ou detalhes de vulnerabilidades exploráveis em issues públicas.

## Reporte privado de vulnerabilidades

Não abra issue pública para uma vulnerabilidade. Use **Report a vulnerability**
na [área de advisories de segurança](https://github.com/Orisema/Orisema-Infrastructure/security/advisories/new).
Inclua versão/commit afetado, impacto, pré-condições e passos mínimos para
reprodução, sem dados ou credenciais reais. O recebimento de um reporte não
implica prazo de correção ou recompensa.

Se o formulário privado não estiver disponível, não publique os detalhes:
avise os mantenedores por um canal privado já estabelecido e aguarde a abertura
do canal de disclosure.

## Política de suporte

| Linha | Estado | Compromisso |
| --- | --- | --- |
| pré-release mais recente | experimental | triagem comunitária, sem SLA |
| builds de `main` | desenvolvimento | sem suporte e sujeitos a mudança |
| qualquer outra versão | não suportada | sem correções prometidas |

Pré-releases podem receber correções somente na próxima pré-release. Não há
backports garantidos. Toda Alpha deve ser usada apenas em laboratórios
descartáveis e sem dados sensíveis.

Princípios: menor privilégio, operações destrutivas com confirmação, registros auditáveis, segredos fora do código e restauração de backups efetivamente testada.

## Artefatos experimentais

Alpha 8 e versões anteriores incluem apenas checksums SHA-256, que detectam
corrupção e troca acidental, mas não autenticam o publicador. A partir da Alpha
9, o workflow foi preparado para assinatura Sigstore keyless dos checksums e
attestations de provenance dos arquivos. Essa garantia só existe quando a
release contém o bundle, a assinatura passa para a identidade exata documentada
e a attestation passa para este repositório; não deve ser inferida para releases
anteriores nem em caso de verificação incompleta.
