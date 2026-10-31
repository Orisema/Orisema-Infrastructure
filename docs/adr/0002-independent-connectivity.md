# ADR 0002 — Conectividade remota independente

- Status: Aceita; registry e transporte implementados, inspeção pendente
- Contexto: Fase 2 — Operação

## Contexto

O Orisema precisa alcançar hosts remotos, inclusive em redes privadas ou atrás de NAT/CGNAT, sem exigir infraestrutura mantida pelos autores ou pela comunidade. SSH sozinho não atravessa redes sem rota acessível.

## Decisão

1. O modo Standalone permanece funcional localmente, sem cadastro externo, plano de controle central, VPN ou relay oficial.
2. A operação remota usa SSH autenticado por chave, com validação estrita da chave do host, privilégios mínimos, operações auditáveis e política fail-closed.
3. A camada de conectividade recebe um destino já alcançável por rede local, VPN administrada pelo usuário, bastion/ProxyJump ou outro transporte configurado pelo operador. O Orisema não exige nem administra uma VPN específica.
4. WireGuard, Tailscale, Headscale e alternativas são integrações opcionais, nunca dependências obrigatórias.
5. Não será desenvolvido ou operado serviço oficial de relay, coordenação de peers, VPN central ou infraestrutura de túnel hospedada pela comunidade Orisema.
6. Primeiro entregar inspeção remota somente leitura; operações mutáveis exigirão plano, confirmação, ownership, journal persistente no host e recuperação conservadora.
7. Credenciais e chaves privadas não entram nos manifestos ou journals. A autenticação SSH não substitui a autorização por operação.

## Consequências

- Hosts atrás de CGNAT podem ser gerenciados se o operador fornecer um caminho de rede funcional, como VPN ou intermediário acessível.
- Não se promete conectividade universal nem travessia automática de NAT sem intermediário.
- O usuário assume a configuração e a disponibilidade do mecanismo de conectividade escolhido.
- Testes devem comprovar independência de serviços externos, verificação de host key, ProxyJump, recusa de acesso indevido e comportamento seguro diante de desconexões.

## Não objetivos

- Hospedar um serviço de conectividade da comunidade.
- Criar protocolo VPN próprio ou exigir agente residente para SSH Standalone.
- Expor portas ou serviços automaticamente à internet.
