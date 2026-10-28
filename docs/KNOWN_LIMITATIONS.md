# Limitações conhecidas — v0.1.0-alpha.1

A Alpha 1 é um laboratório local para hosts Linux descartáveis. **Não é
indicada para produção** e não possui garantia de estabilidade ou continuidade.

## Escopo suportado

- Um host Linux local com Podman; Ubuntu 22.04/Podman 3.4.4 e Ubuntu
  24.04/Podman 4.9.3 compõem a matriz prática atual.
- Binários Linux AMD64 e ARM64. A compilação ARM64 é verificada, mas o quick
  start ainda não é executado em hardware ARM64 no CI.
- Apenas campos e comandos documentados em `docs/COMPATIBILITY.md`.

## Fora do escopo

- Produção, alta disponibilidade, múltiplos nós ou operação remota.
- Docker, Windows e macOS.
- Atualização ou substituição de contêiner existente. `DRIFT` e `REVIEW`
  bloqueiam o deploy e exigem investigação manual.
- Rollback de aplicação ou dados, backup/restore e migrações de volume.
- Gerenciamento de secrets, variáveis de ambiente, volumes, limites de
  recursos, redes personalizadas e permissões avançadas no manifesto.
- Identidade autenticada do operador ou journal resistente a adulteração.
- Assinatura criptográfica e atestação de proveniência dos artefatos; há apenas
  checksums SHA-256.

## Comportamentos intencionais

- Portas publicadas usam o comportamento padrão do Podman e podem escutar em
  todas as interfaces. Use somente laboratório isolado.
- `NOOP` compara os campos declarados suportados; não prova saúde ao vivo nem
  equivalência completa da configuração.
- Recovery executa somente `START`, requer reserva/confirmação fresca e não
  promete recuperar aplicações unhealthy.
- Uma implantação com vários serviços não é transacional. Se um serviço
  posterior falhar, serviços anteriores já criados não são revertidos.

Veja o [plano Alpha](ALPHA_ROADMAP.md) para o que está proposto nas versões
seguintes; itens futuros não são contratos da Alpha 1.
