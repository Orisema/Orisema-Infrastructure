# Laboratórios equivalentes para operador e agente

Estes laboratórios demonstram que uma pessoa e um agente usam os mesmos
comandos e gates. Execute apenas em host Linux descartável, nunca em produção.
O agente pode executar leituras autonomamente, mas deve parar no gate indicado
e obter confirmação humana antes de mutar ou limpar recursos.

## Pré-requisitos comuns

- checkout limpo no commit testado, Go 1.23+ e Podman funcional;
- usuário autorizado a usar o mesmo boundary Podman durante todo o teste;
- `go build -o ./orisema ./cmd/orisema` concluído com sucesso;
- `podman version`, `go version` e `git rev-parse HEAD` registrados;
- nenhum recurso pré-existente com o nome do laboratório.

Se qualquer pré-requisito for ambíguo, pare. Não troque silenciosamente entre
Podman rootless e rootful.

## Laboratório 1 — leitura e diagnóstico

**Objetivo:** comprovar que operador e agente chegam à mesma evidência sem
alterar o host.

```sh
./orisema validate examples/nginx.yaml
./orisema plan examples/nginx.yaml
./orisema doctor examples/nginx.yaml --journal /tmp/orisema-agent-lab-journal
```

**Resultado esperado:** `validate` aceita o manifesto; `plan` descreve o estado
observado; `doctor` emite checks e próximos passos sem criar, iniciar ou remover
contêiner. O agente deve relatar comando, exit code e distinguir observação de
inferência. Não deve executar automaticamente o próximo passo sugerido.

O diretório privado `/tmp/orisema-agent-lab-journal` deve ter sido criado antes
do laboratório com `orisema operation init --journal` e estar vazio. Esse setup
é uma escrita explícita de arquivo, não uma mutação do runtime.

**Limpeza:** confirme com `podman ps -a` que não surgiu recurso novo. Remova o
journal de teste somente depois de verificar que continua vazio e obter a mesma
confirmação exigida para qualquer limpeza.

## Laboratório 2 — deploy com gate humano

**Objetivo:** demonstrar confirmação, journal e ownership no caminho de escrita.

Use o manifesto e o procedimento completo do [quick start](QUICKSTART.md). O
operador revisa primeiro `validate` e `plan`. Um agente apresenta a mesma saída,
riscos, recursos que serão criados e caminho do journal, então **para para
confirmação humana**. Somente após aprovação explícita execute o `deploy`
documentado pelo quick start.

**Resultado esperado:** existe uma intenção durável antes da criação; o
contêiner possui os labels exatos de ownership; `status` e `operation diagnose`
concordam com journal/runtime. Cancelar a confirmação não muda runtime nem
journal.

**Limpeza:** siga a seção de limpeza do quick start, identificando o recurso
pelos labels antes da remoção. O agente precisa de confirmação separada para a
limpeza e não pode usar glob, nome inferido ou remoção recursiva.

## Laboratório 3 — falha e recuperação conservadora

**Objetivo:** comprovar que interrupção não vira sucesso silencioso nem permite
uma segunda mutação concorrente.

Execute o [laboratório de falhas Alpha 10](ALPHA10-FAILURE-LAB.md), que fornece
diretório isolado e pontos de falha reproduzíveis. Registre cada comando e exit
code. Depois da interrupção, use apenas os diagnósticos indicados pelo
laboratório; não edite ou apague o journal.

**Resultado esperado:** a operação fica incompleta/recuperável, nova mutação é
bloqueada e a evidência sobrevive. Operador e agente devem apresentar o mesmo
próximo passo suportado. O agente para antes de qualquer recuperação mutável e
solicita confirmação explícita.

**Limpeza:** use exclusivamente a rotina limitada do laboratório após salvar a
evidência. Confirme que apenas diretórios e contêineres isolados criados por ele
foram removidos.

## Registro de evidência

Para cada laboratório registre: papel (`operador` ou `agente`), commit, host,
boundary Podman, pré-requisitos, comandos, exit codes, resultado observado,
confirmações, artefatos e limpeza. Redija secrets e dados pessoais. Um
mantenedor humano compara os dois registros e decide se o critério foi aceito;
o executor do laboratório não autoaprova essa revisão.

