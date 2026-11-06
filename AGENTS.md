# Guia para agentes e contribuidores

Este arquivo se aplica a todo o repositório. Automação assistida por IA segue o
mesmo contrato de um operador humano; ela não recebe autoridade adicional.

## Regras obrigatórias

1. Não invente resultados. Diferencie claramente fato observado, inferência e
   trabalho ainda não executado. Nunca descreva um teste, revisão ou ambiente
   como aprovado sem evidência correspondente.
2. Preserve o comportamento fail-closed. Identidade, ownership, estado,
   journal, autenticação ou evidência ambígua devem bloquear a operação sem
   apagar o material necessário ao diagnóstico.
3. Diagnóstico não é autorização. `plan`, `doctor`, `diagnose` e comandos de
   leitura podem orientar o próximo passo, mas não autorizam uma mutação.
4. Não contorne confirmação, journal, lock, revalidação ou recuperação. Um
   agente deve mostrar o plano e obter a mesma aprovação exigida no caminho
   manual antes de uma ação destrutiva ou externa.
5. Preserve secrets. Não copie valores, credenciais, locators privados ou
   saída sensível para prompts, logs, planos, testes, commits ou issues.
6. Use somente comandos públicos documentados em `docs/CLI-INVENTORY.md`. Não
   trate arquivos internos do journal como uma API para edição manual.
7. Limite a mudança ao problema aprovado, preserve alterações do usuário e
   proponha RFC/ADR antes de ampliar contratos ou fronteiras de confiança.

## Evidência mínima de uma contribuição

Preencha o template de PR com problema, módulos, contratos, riscos, efeitos,
recuperação e comandos realmente executados. Teste caminhos positivos e
negativos proporcionais ao risco. Mudanças em concorrência ou persistência
exigem race detector; mudanças de runtime ou SSH exigem laboratório isolado.

Uma revisão humana é obrigatória antes do merge. O agente pode preparar a
evidência, mas não pode marcar em nome do mantenedor a aprovação humana.

Consulte o [manual do mantenedor](docs/MAINTAINER-MANUAL.md) e os
[laboratórios operador/agente](docs/OPERATOR-AGENT-LABS.md).

