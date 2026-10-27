# Segurança

**Não existe versão pronta para produção.**

Não publique credenciais, tokens, dados pessoais ou detalhes de vulnerabilidades exploráveis em issues públicas.

Antes da primeira versão, a organização deverá configurar um canal privado de reporte de vulnerabilidades, preferencialmente o recurso de private vulnerability reporting do GitHub.

Princípios: menor privilégio, operações destrutivas com confirmação, registros auditáveis, segredos fora do código e restauração de backups efetivamente testada.

## Artefatos experimentais

Releases incluem checksums SHA-256 e são publicadas por um workflow restrito a
tags, usando apenas o `GITHUB_TOKEN` com permissão de conteúdo. Checksums
detectam corrupção e troca acidental, mas não substituem assinatura criptográfica
nem atestação de proveniência. Esses mecanismos são melhorias futuras e não
devem ser presumidos no Alpha 1.
