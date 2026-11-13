# Limitações conhecidas — desenvolvimento v0.1.0-alpha.10

A branch `main` é um protótipo operacional para hosts Linux descartáveis. **Não é
indicada para produção** e não possui garantia de estabilidade ou continuidade.

## Escopo suportado

- Um host Linux local com Podman; Ubuntu 22.04/Podman 3.4.4 e Ubuntu
  24.04/Podman 4.9.3 compõem a matriz prática atual.
- Binários Linux AMD64 e ARM64. A compilação ARM64 é verificada, mas o quick
  start ainda não é executado em hardware ARM64 no CI.
- Apenas campos e comandos documentados em `docs/COMPATIBILITY.md`.

## Fora do escopo

- Produção, alta disponibilidade ou orquestração de múltiplos nós.
- A operação remota experimental é somente leitura: status e cauda fixa de 100
  linhas de logs. Toda mutação remota continua indisponível.
- ProxyJump foi validado em topologia local com dois servidores SSH. CGNAT,
  bastions externos e produtos VPN específicos ainda não foram testados.
- Docker, Windows e macOS.
- O fluxo experimental Alpha 5 atualiza apenas um serviço elegível por vez;
  `deploy` continua bloqueando `DRIFT` e `REVIEW`.
- Update configurado aceita somente drift exclusivo de imagem. Mudanças
  declarativas de ambiente, secrets ou redes não são aplicadas in-place. O
  desenvolvimento da Beta 1 permite persistir somente uma intenção de revisão
  para drift exclusivo de configuração pública em serviço sem secrets ou
  redes. A intenção pode ser autorizada após revalidação e confirmação exata,
  e `apply` pode preservar a unidade anterior e criar um candidato. Verify,
  verify pode decidir `COMPLETED` ou `RECOVERY_REQUIRED`; rollback e cleanup
  permanecem bloqueados.
- Rollback de dados, backup/restore e migrações de volume. O rollback público
  existente restaura somente o container da aplicação preservado pelo update.
- Backup/restore protege somente configuração validada e histórico operacional
  inerte. Nenhum dado persistente, secret ou credencial é protegido.
- `v1alpha3` resolve e monta referências Podman e arquivos locais validados por
  identidade e gerencia o ciclo de vida Podman sem revelar valores. A Alpha 8
  cria candidatos, revalida esses bindings no verify e preserva a unidade
  anterior para rollback manual.
- `v1alpha4` entrega configuração explicitamente pública e conecta CREATE a
  redes Podman externas `bridge` por identidade revalidada, sem gerenciar o
  ciclo de vida dessas redes. O candidato, verify e rollback preservam e
  revalidam esses campos; secrets em variáveis de ambiente, volumes, limites
  de recursos e permissões avançadas não têm suporte.
- Aliases DNS de rede não fazem parte do contrato. O laboratório MixIdentity
  aplica o alias isolado `identity-app` explicitamente entre dois deploys; isso
  não representa gerenciamento de alias pelo Orisema.
- Identidade autenticada do operador ou journal resistente a adulteração.
- O audit local remoto é durável e privado, mas não é assinado nem resistente a
  adulteração por um usuário com acesso ao arquivo.
- O journal operacional local e o audit remoto não são assinados nem
  resistentes a adulteração por um usuário com acesso aos arquivos. Artefatos
  de release da Alpha 9 em diante possuem verificação separada por SHA-256,
  Sigstore keyless e GitHub artifact attestation.

## Comportamentos intencionais

- Manifestos legados `v1alpha1` preservam binds em todas as interfaces;
  `v1alpha2` exige `hostIP` loopback explícito. Use somente laboratório isolado.
- `NOOP` compara os campos declarados suportados; não prova saúde ao vivo nem
  equivalência completa da configuração.
- Recovery executa somente `START`, requer reserva/confirmação fresca e não
  promete recuperar aplicações unhealthy.
- Uma implantação com vários serviços não é transacional. Se um serviço
  posterior falhar, serviços anteriores já criados não são revertidos.

Veja o [inventário verificável da CLI](CLI-INVENTORY.md) para efeitos e
recuperação de cada comando e o [plano Alpha](ALPHA_ROADMAP.md) para itens
futuros, que não constituem contratos implementados.
