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
| `v0.1.0-alpha.1` | experimental | triagem comunitária, sem SLA |
| builds de `main` | desenvolvimento | sem suporte e sujeitos a mudança |
| qualquer outra versão | não suportada | sem correções prometidas |

Pré-releases podem receber correções somente na próxima pré-release. Não há
backports garantidos. A Alpha 1 deve ser usada apenas em laboratórios
descartáveis e sem dados sensíveis.

Princípios: menor privilégio, operações destrutivas com confirmação, registros auditáveis, segredos fora do código e restauração de backups efetivamente testada.

## Artefatos experimentais

Releases incluem checksums SHA-256 e são publicadas por um workflow restrito a
tags, usando apenas o `GITHUB_TOKEN` com permissão de conteúdo. Checksums
detectam corrupção e troca acidental, mas não substituem assinatura criptográfica
nem atestação de proveniência. Esses mecanismos são melhorias futuras e não
devem ser presumidos no Alpha 1.
