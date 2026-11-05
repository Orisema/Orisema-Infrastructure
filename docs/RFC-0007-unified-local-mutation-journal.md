# RFC-0007 — Journal unificado para mutações locais

Status: **proposto para revisão**  
Issues: [#144](https://github.com/Orisema/Orisema-Infrastructure/issues/144),
[#145](https://github.com/Orisema/Orisema-Infrastructure/issues/145)

## Resumo

A Alpha 10 tornará o journal uma pré-condição explícita para toda mutação local
suportada. `deploy`, `start` e `stop` passarão a admitir e registrar a operação
antes de alterar o runtime, usando a mesma exclusão por aplicação já aplicada
a update, rollback e recovery.

O objetivo não é prometer transações ou recuperação automática. É impedir que
uma mutação nova atravesse evidência incompleta, preservar intenção e progresso
quando houver interrupção e fornecer um próximo passo seguro ao operador.

## Problema

O store atual rejeita uma nova operação quando já existe uma operação
incompleta para a mesma aplicação. Entretanto, os caminhos legados `deploy`,
`start` e `stop` ainda alteram o runtime sem criar uma operação no store. Uma
consulta anterior à mutação seria insuficiente: dois processos poderiam
consultar o mesmo estado livre e prosseguir simultaneamente.

Além disso, criar implicitamente um diretório de estado em um caminho digitado
incorretamente esconderia erro operacional. A CLI precisa distinguir
inicialização deliberada, inspeção somente leitura e uso mutável de um store
existente.

## Decisão

### Interface da CLI

A inicialização será explícita:

```text
orisema operation init --journal /caminho/absoluto
```

As mutações locais exigirão `--journal`:

```text
orisema deploy --journal /caminho/absoluto app.yaml
orisema start --journal /caminho/absoluto app.yaml web
orisema stop --journal /caminho/absoluto app.yaml web
```

Não haverá descoberta por diretório atual, variável de ambiente ou caminho
padrão. O caminho deverá ser absoluto. A ausência da opção, um store ausente ou
um store inválido produzirão erro antes de qualquer chamada mutável ao runtime.
Comandos somente leitura não ganharão essa pré-condição.

Essa é uma mudança incompatível deliberada durante o período pré-beta. A forma
antiga falhará com erro de uso e instrução de migração; ela não terá um modo de
compatibilidade que contorne o journal.

### Inicialização e fronteira do store

Somente `operation init` poderá criar a raiz do store e seu lock. A
inicialização deverá:

- rejeitar links simbólicos e caminhos ambíguos;
- criar uma raiz privada pertencente ao usuário efetivo;
- criar e validar `.store.lock` como arquivo regular privado;
- ser idempotente apenas quando a raiz existente já satisfizer o contrato; e
- não ler manifesto nem tocar no runtime.

Os comandos mutáveis abrirão o store em modo gravável existente, sem criar ou
reparar estruturas. Corrupção, permissões inseguras, ownership inesperado ou
snapshot incompatível bloquearão a operação inteira.

### Admissão atômica

Depois do planejamento e da confirmação exata, a CLI repetirá todas as
validações mutáveis e admitirá a operação sob o lock global do store. A admissão
deverá, em uma única seção crítica:

1. carregar e validar todos os snapshots;
2. rejeitar qualquer operação incompleta da mesma aplicação;
3. atribuir um identificador único; e
4. persistir atomicamente a intenção em `PENDING`.

O lock não permanecerá preso durante chamadas potencialmente longas ao Podman.
O snapshot incompleto recém-criado passa a ser o bloqueio durável contra outro
processo. Uma operação de outra aplicação pode prosseguir, desde que o store
inteiro esteja íntegro.

A confirmação acontece antes da admissão para que uma resposta negativa não
crie evidência operacional incompleta. Imediatamente depois da confirmação, a
CLI replaneja e revalida sob a fronteira de admissão; mudança de plano, runtime,
ownership ou bindings exige uma nova execução e confirmação.

### Conteúdo persistido

O registro de mutação conterá somente metadados necessários para diagnóstico e
recuperação:

- tipo `DEPLOY`, `START` ou `STOP`;
- aplicação e serviços-alvo;
- fingerprint do manifesto normalizado e do plano confirmado;
- modo rootless/rootful e identidades observadas de recursos;
- etapas planejadas, etapa corrente e resultados sem valores de configuração;
- timestamps, estado da máquina e causa estruturada de interrupção.

Valores de secrets e de configuração pública não serão persistidos. Caminhos
de origem sensíveis serão reduzidos à identidade já prevista pelos respectivos
contratos; logs e erros seguirão a mesma regra.

### Execução e estados

Após a admissão, a execução usa os limites duráveis existentes:

```text
PENDING -> PREPARING -> APPLYING -> VERIFYING -> COMPLETED
```

Cada transição é sincronizada antes da ação correspondente. `COMPLETED` só é
gravado depois de uma inspeção nova confirmar o estado pretendido. Erro,
cancelamento, sinal, timeout ou resultado não verificável após a admissão deixa
uma operação incompleta e bloqueante. Não haverá limpeza otimista nem marcação
de sucesso para liberar o gate.

Para `start` e `stop`, a verificação compara a identidade original e o estado
final do serviço. Para `deploy`, cada criação é precedida por uma etapa
persistida, e o sucesso de cada serviço é registrado antes do próximo. Falha
parcial preserva os contêineres já criados e a evidência; a Alpha 10 não promete
rollback automático de um deploy de vários serviços.

### Recuperação

`doctor` e `operation inspect` permanecem somente leitura. Eles devem explicar
qual operação bloqueia a aplicação, a última etapa durável e o comando seguro
seguinte, sem inferir que uma chamada ao runtime concluída implica saúde.

No primeiro incremento, `DEPLOY`, `START` e `STOP` interrompidos não terão
retomada ou descarte genérico. A recuperação mutável só será adicionada para um
estado quando identidade, ownership, efeito pretendido e pós-condição puderem
ser revalidados objetivamente. Até lá, a evidência é preservada e a aplicação
permanece bloqueada.

## Ordem de validação

Antes de exibir confirmação:

1. parse e validação estrita dos argumentos;
2. abertura e validação somente leitura do store existente;
3. carregamento e validação do manifesto;
4. inspeção do runtime e resolução de bindings;
5. construção do plano exato e verificação de conflitos conhecidos.

Depois da confirmação, todos os itens que podem mudar são revalidados, a
operação é admitida atomicamente e só então começa a mutação.

## Compatibilidade e rollout

A implementação será dividida em incrementos revisáveis:

1. abertura gravável de store existente e admissão atômica testada;
2. `operation init` e migração da sintaxe da CLI;
3. journal de `start` e `stop`, incluindo sinais e resultado ambíguo;
4. journal por etapas de `deploy` e falha parcial;
5. integração com `doctor`, documentação e laboratórios.

Leitores novos continuarão aceitando snapshots íntegros das versões anteriores.
Nenhum migrador reescreverá automaticamente journals antigos. Binários antigos
não são considerados seguros para operar um store depois que ele contiver tipos
da Alpha 10; o instalador e a documentação deverão advertir sobre downgrade.

## Gates de aceitação

- Nenhuma chamada mutável ao runtime sem store explícito, íntegro e existente.
- Concorrência para a mesma aplicação admite exatamente uma operação.
- Corrupção em qualquer snapshot relevante falha antes da mutação.
- Confirmação cancelada não cria operação nem altera o runtime.
- Mudança entre plano, confirmação e admissão falha fechado.
- `SIGINT`, `SIGTERM` e encerramento abrupto deixam evidência diagnosticável.
- Falha parcial de deploy registra o último limite durável sem remover recursos.
- Rootless e rootful não compartilham evidência ou identidade por acidente.
- Nenhum canário secreto aparece em journal, plano, erro ou log.
- Testes práticos cobrem Ubuntu e Fedora antes da release.

## Fora do escopo

- transação ou rollback automático de deploy com múltiplos serviços;
- recuperação genérica, descarte forçado ou edição manual de snapshots;
- mutação remota, multi-host ou coordenação distribuída;
- lifecycle de secrets, redes externas ou dados persistentes; e
- formato JSON público para automação antes de existir envelope versionado.

