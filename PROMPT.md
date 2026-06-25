# PROMPT.md — Registro do uso da IA

Este arquivo registra como a ferramenta de IA foi utilizada ao longo da
atividade, seguindo as etapas do roteiro, com resumo das respostas, sugestões
aceitas/rejeitadas e decisões técnicas tomadas pelo estudante.

## Ambiente e ferramenta

- **Ferramenta de IA:** Claude (Claude Code — CLI), modelo Claude Opus 4.x.
- **Linguagem:** Go **1.26.2** (`darwin/amd64`).
- **Máquina:** macOS, CPU Intel Core i7-4770HQ (4 núcleos físicos / 8 threads).
- **Dataset:** *A Christmas Carol* (Charles Dickens), ~160 KB, ~29 mil palavras.

> Observação geral: a IA foi usada como apoio (compreender o problema, comparar
> estratégias, gerar rascunhos, revisar criticamente e analisar tempos). Cada
> sugestão foi avaliada, testada e, quando necessário, corrigida. Os pontos em
> que a IA errou ou propôs algo subótimo estão registrados em cada etapa.

---

## Etapa 1 — Compreensão do problema

**Prompt usado:**
> Quero implementar em Go um programa de contagem de frequência de palavras em um
> arquivo texto. O programa deve ter uma versão sequencial e uma versão
> concorrente. Explique o problema, as principais decisões de implementação e os
> cuidados necessários para garantir que as duas versões produzam exatamente o
> mesmo resultado.

**Resumo da resposta:** o problema é ler o texto, normalizar (minúsculas,
remover pontuação, descartar palavras curtas) e contar. As decisões-chave são:
como tokenizar, como normalizar e onde armazenar as frequências (`map[string]int`).
Para as versões coincidirem, ambas devem aplicar **exatamente as mesmas regras**
e a ordem de processamento não pode influenciar o resultado (a contagem é
comutativa/associativa, então dividir e recombinar é seguro).

**Decisões da etapa:**
- **Leitura:** arquivo inteiro em memória (`os.ReadFile`) — cabe folgado.
- **Separação:** dividir por sequências de letras (pontuação/espaço = separador).
- **Normalização:** `strings.ToLower` + descartar < 3 caracteres.
- **Armazenamento:** `map[string]int`.
- **Referência:** a sequencial define o resultado "correto"; a concorrente é
  validada contra ela.
- **Riscos da concorrente:** escrever num mapa compartilhado sem proteção (panic
  de "concurrent map writes"), e divergência por tokenização inconsistente.
- **Decisão crítica antecipada:** as duas versões compartilharão **a mesma
  função de contagem** para impossibilitar divergência de regras.

---

## Etapa 2 — Investigação das alternativas concorrentes

**Prompt usado:**
> Em Go, quais são as principais formas de implementar uma contagem concorrente
> de frequência de palavras em um arquivo texto? Compare alternativas como
> goroutines com channels, goroutines com WaitGroup, mapa global com Mutex,
> sync.Map, mapas locais com redução final e pool de workers. Para cada
> alternativa, indique vantagens, desvantagens, riscos de condição de corrida e
> custo de sincronização.

**Alternativas sugeridas e avaliação:**

| Alternativa | Vantagem | Desvantagem / risco de corrida | Custo de sincronização |
|---|---|---|---|
| Mapa global + `Mutex` | simples de explicar | **contenção altíssima** (todo `++` disputa o lock); sem lock → corrida/panic | alto (lock no caminho quente) |
| `sync.Map` | sem lock explícito | feito p/ muita leitura; aqui é escrita intensa → lento; API verbosa | médio/alto |
| Channel → agregador único | um só dono do mapa (sem corrida) | agregador serial vira gargalo; comunicação palavra a palavra é cara | alto (comunicação) |
| `WaitGroup` + mapas locais + redução | **sem corrida, sem contenção** | usa mais memória (N mapas); fusão final O(distintas) | **mínimo** (1 barreira) |
| Pool de workers + blocos | controla nº de goroutines | precisa cortar o texto sem partir palavra | mínimo |

**Observações críticas do estudante:**
- A IA tendeu a apresentar "mapa global com mutex" como solução "natural". Isso é
  um **anti-padrão de desempenho** aqui: a operação protegida (`counts[w]++`) é
  justamente a mais frequente, então o lock serializa quase todo o trabalho.
- `sync.Map` foi corretamente sinalizado como inadequado para escrita intensa.
- A combinação **pool de workers + mapas locais + redução** apareceu como a de
  menor custo de sincronização e sem risco de corrida — candidata forte.

---

## Etapa 3 — Escolha da estratégia

**Prompt usado:**
> Considerando o problema de contar frequência de palavras em um arquivo texto,
> qual estratégia concorrente em Go parece mais adequada? Justifique considerando
> correção, simplicidade, risco de condição de corrida, custo de sincronização e
> desempenho.

**Resposta resumida:** a IA recomendou **pool de workers com mapas locais e
redução final**, por não ter estado compartilhado mutável (correção trivial,
sem corrida) e por evitar contenção.

**Decisão (combinando elementos):** **aceita**, com um reforço próprio do
estudante — em vez de só "concordar", decidi **reutilizar a mesma função de
contagem (`countInto`) nas duas versões**. Assim a equivalência sequencial ↔
concorrente passa a valer *por construção*, não por coincidência de
implementação. Também fixei o corte dos blocos **em espaço em branco** para
nunca partir uma palavra (a IA havia descrito o corte de forma vaga).

---

## Etapa 4 — Planejamento do programa

**Prompt usado:**
> Proponha a estrutura de um programa em Go para resolver esse problema. (...)
> Não escreva o código ainda; descreva apenas as principais funções necessárias.

**Estrutura proposta pela IA** (próxima às assinaturas do roteiro):
`readFile`, `normalizeWord`, `countWordsSequential`, `countWordsConcurrent`,
`mergeCounts`, `sameCounts`, `topN`.

**Ajustes feitos pelo estudante:**
- Adicionada `countInto(counts, text)` como **função compartilhada** de
  tokenização/contagem (não estava na lista original). É o que garante regras
  idênticas.
- Adicionada `splitIntoChunks(text, n)` para a divisão segura em blocos.
- `topN` passou a devolver `[]wordCount` (par palavra/contagem) com **desempate
  alfabético**, para saída determinística.
- `normalizeWord` ficou enxuta (só `ToLower`), pois a remoção de pontuação é
  feita na tokenização.

---

## Etapa 5 — Implementação da versão sequencial

**Prompt usado:**
> Implemente em Go a versão sequencial do conta-palavras. (...) receber o caminho
> por argumento, ignorar palavras com menos de 3 caracteres, converter para
> minúsculas, remover pontuação simples, medir o tempo e imprimir as 20 mais
> frequentes.

**Verificações (todas OK):** compila; recebe o arquivo por `os.Args[1]`; trata
erro de leitura (mensagem + `os.Exit(1)`); normalização confere; imprime o tempo
sequencial; Top 20 ordenado por contagem decrescente.

**Problema encontrado e corrigido:** a primeira ideia de filtrar por tamanho
usando `len(word)` (bytes) estava **errada para acentos** — `"é"` tem 2 bytes e
*não* seria descartada como deveria. **Correção:** usar
`utf8.RuneCountInString(word)` (contagem por runes).

---

## Etapa 6 — Revisão crítica da versão sequencial

**Prompt usado:**
> Revise o código sequencial abaixo. Verifique correção, normalização,
> tratamento de erros, ordenação e medição de tempo. Sugira melhorias, mas ainda
> não implemente a concorrente.

**Sugestões da IA → decisão:**
- Desempate alfabético no `topN` → **aceita** (determinismo).
- Contar runes em vez de bytes → **aceita** (corrige acentos).
- Usar `bufio.Scanner` em vez de ler o arquivo inteiro → **rejeitada**: o arquivo
  é pequeno e ler tudo de uma vez simplifica e deixa as duas versões partindo do
  mesmo texto em memória.
- Trocar `os.Exit` por `log.Fatal` → **rejeitada** (estética; `Fprintln` +
  `os.Exit(1)` é suficiente e explícito).

---

## Etapa 7 — Teste de correção com entrada pequena

**Prompts usados:**
> Crie um pequeno arquivo de teste para validar a contagem (...) palavras
> repetidas, maiúsculas/minúsculas, pontuação e palavras com menos de 3
> caracteres. Informe o mapa esperado.

> Adicione ao programa uma forma simples de testar a versão sequencial usando um
> arquivo pequeno com resultado esperado conhecido.

**Entrada usada (`sample.txt`):**
```
Casa, casa! A casa é azul.
Árvore; árvore? verde.
Go go Go. IA é útil, mas IA erra.
```

**Resultado esperado:**
`casa:3, árvore:2, azul:1, verde:1, útil:1, mas:1, erra:1`
(`a`, `é`, `go`, `ia` são ignoradas por terem menos de 3 caracteres).

**Resultado produzido:** idêntico ao esperado. Implementado como teste Go
(`go test`) em `main_test.go` (`TestSequentialSmall`), em vez de um modo `-test`
no `main`, por ser a forma idiomática e automatizável de testar em Go.

**Correções:** nenhuma adicional — o teste passou de primeira após a correção de
runes da Etapa 5.

---

## Etapa 8 — Implementação da versão concorrente

**Prompt usado (adaptado à estratégia):**
> A partir da versão sequencial, implemente uma versão concorrente em Go (...)
> usando a estratégia escolhida; deve produzir o mesmo mapa, medir o tempo
> concorrente e permitir configurar o número de workers.

**Estratégia implementada:** `splitIntoChunks` divide o texto em N blocos
(cortando em espaço em branco); cada goroutine conta seu bloco em um mapa local
(`countInto`); `sync.WaitGroup` sincroniza; `mergeCounts` faz a redução final.
Nº de workers vem do 2º argumento da linha de comando (padrão = `runtime.NumCPU`).

**Alterações feitas pelo estudante sobre o rascunho da IA:**
- O rascunho da IA cortava os blocos por índice fixo (`len/N`), o que **partiria
  palavras** na fronteira → corrigido para avançar o corte até o próximo espaço.
- O rascunho usava `rune(text[i])` para achar a fronteira, o que decodifica
  errado UTF-8 multibyte → troquei por checagem de **espaço ASCII por byte**
  (espaços são < 0x80, nunca caem no meio de uma sequência UTF-8).
- Garanti que cada goroutine escreve em `locals[i]` (índice exclusivo), evitando
  qualquer escrita concorrente.

**Problemas de compilação/execução:** o programa compilou (`go build` OK) e
`go vet` não acusou nada.

---

## Etapa 9 — Comparação dos resultados

**Prompt usado:**
> Adicione ao programa uma função para comparar o mapa da versão sequencial com o
> da versão concorrente (...) mesmas palavras, mesmas frequências; imprima se são
> iguais ou diferentes.

**Implementação:** `sameCounts(a, b)` compara o **mapa completo** (tamanho +
cada par palavra/contagem). O `main` imprime `Resultados iguais: sim/não`.

**Resultado:** `Resultados iguais: sim` no dataset e em `sample.txt`. O teste
`TestSeqEqualsConcurrentDataset` confirma a igualdade para 1/2/3/4/8/16 workers.

---

## Etapa 10 — Análise de condições de corrida

**Prompt usado:**
> Analise a versão concorrente (...) Existe condição de corrida? Alguma estrutura
> é escrita por mais de uma goroutine? Há risco de deadlock? A sincronização é
> necessária? Existe contenção desnecessária?

**Avaliação (específica, citando o código):**
- **Sem corrida:** cada goroutine escreve só no seu mapa local e em `locals[i]`
  (índice próprio); nada mutável é compartilhado durante a contagem.
- **Sem deadlock:** o único ponto de sincronização é `wg.Wait()`; cada goroutine
  faz `defer wg.Done()`.
- **Sincronização necessária e mínima:** o `WaitGroup` é indispensável; não há
  `Mutex` porque não há recurso compartilhado — adicioná-lo seria contenção
  desnecessária.
- **Sem contenção** na fase de contagem.
- **Confirmação empírica:** `go test -race ./...` passou **sem** reportar corrida.

**Decisão:** manter a estratégia. A IA, ao revisar, chegou a sugerir "proteger o
mapa com mutex por segurança" — **rejeitado**, pois não há mapa compartilhado;
seria sincronização inútil que só adicionaria contenção.

---

## Etapa 11 — Testes com diferentes configurações

Executado `./wordcount AChristmasCarol_CharlesDickens_English.txt {1,2,4,8}`,
3 execuções cada. Tempos (concorrente, medição do `main`):
1 worker ≈ 4,2 ms · 2 workers ≈ 2,7 ms · 4 workers ≈ 2,3 ms · 8 workers ≈ 3,4 ms.

Também rodado `go test -bench` (medição isolada, 200 repetições) — ver Etapa 12.

---

## Etapa 12 — Análise de desempenho

**Prompt usado:**
> Analise os tempos de execução obtidos. Explique por que a versão concorrente
> foi mais rápida/mais lenta/equivalente (...). Considere tamanho do arquivo,
> custo de goroutines, sincronização, comunicação, divisão do trabalho, núcleos e
> overhead da combinação.

**Tempos informados à IA (benchmark isolado):** sequencial 3,72 ms; concorrente
1w 4,42 ms · 2w 2,99 ms · 4w 2,75 ms · 8w 4,78 ms.

**Análise da IA (resumo):** ganho modesto porque o arquivo é pequeno e o trabalho
útil (~3–4 ms) é comparável ao overhead de criar goroutines + dividir + fundir;
o ótimo em 4 workers coincide com os 4 núcleos físicos; 8 workers pioram por
oversubscription (4 núcleos reais) e mais mapas a fundir.

**Avaliação crítica do estudante (ponto que a IA inicialmente não levantou):**
na medição do `main` a sequencial roda **sempre primeiro** e a concorrente
**depois**, então a segunda se beneficia de cache/alocador "quentes" — por isso
no `main` até a concorrente com 1 worker "parece" mais rápida que a sequencial,
o que o benchmark isolado **desmente** (1 worker é, na verdade, mais lento). Isto
é um **viés de ordem da medição**. Conclusão: usar os números do benchmark para
conclusões, e tratar os do `main` como ilustrativos.

---

## Etapa 13 — Registro no README

**Prompt usado:**
> Com base na implementação e nos tempos, ajude a escrever um README.md curto
> explicando a estratégia, as alternativas, a comparação de resultados, a medição
> de tempo, o ganho de desempenho e os cuidados contra condição de corrida.

O `README.md` foi escrito e **revisado criticamente** pelo estudante — em
especial a seção de desempenho, para incluir o viés de ordem e deixar claro que
o ganho real é modesto (e até negativo com 1 ou 8 workers).

---

## Resumo: sugestões aceitas, rejeitadas e correções

**Aceitas:**
- Estratégia de pool de workers + mapas locais + redução final.
- Desempate alfabético no `topN`; contagem por runes; `WaitGroup`.

**Rejeitadas:**
- Mapa global com `Mutex` / `sync.Map` (contenção; inadequado a escrita intensa).
- "Proteger o mapa com mutex por segurança" na versão concorrente (não há mapa
  compartilhado — seria contenção inútil).
- `bufio.Scanner` e `log.Fatal` (preferência por ler tudo + `os.Exit`).

**Erros/problemas gerados pela IA e corrigidos pelo estudante:**
1. Filtrar tamanho por **bytes** (`len`) — quebrava acentos; corrigido para runes.
2. Cortar blocos por índice fixo — **partia palavras**; corrigido para cortar em
   espaço em branco.
3. Usar `rune(text[i])` na fronteira — decodificação UTF-8 incorreta; corrigido
   para checagem de espaço ASCII por byte.
4. Não apontar, de início, o **viés de ordem** da medição de tempo — levantado e
   documentado pelo estudante.

**Decisões técnicas principais:**
- Reutilizar `countInto` nas duas versões → equivalência por construção.
- Medir **apenas a contagem** (leitura fora da medição), de forma equivalente.
- Validar com `sample.txt` (resultado conhecido) e com `-race` antes de confiar
  na versão concorrente.
