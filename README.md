# Contagem de Frequência de Palavras em Go — Sequencial vs. Concorrente

Programa em Go que conta a frequência de palavras em um arquivo texto e exibe as
20 palavras mais frequentes. Implementa **duas versões** da contagem — uma
**sequencial** (referência) e uma **concorrente** — mede o tempo de cada uma e
**compara o mapa completo** de frequências para garantir que produzem o mesmo
resultado.

Atividade da disciplina, desenvolvida com apoio de uma ferramenta de IA. O
registro completo do uso da IA (prompts, respostas, decisões, sugestões aceitas
e rejeitadas) está em [`PROMPT.md`](PROMPT.md).

---

## Problema

Dado um arquivo `.txt`, contar quantas vezes cada palavra aparece, aplicando as
seguintes regras de processamento:

- converter todas as palavras para minúsculas;
- remover pontuação simples;
- ignorar palavras com menos de 3 caracteres;
- contar a frequência de cada palavra (`map[string]int`);
- apresentar as 20 palavras mais frequentes.

A versão sequencial serve de **referência de correção**: a versão concorrente
precisa produzir **exatamente o mesmo mapa**.

---

## Compilação e execução

Requisito: Go instalado (desenvolvido e testado com **Go 1.26.2**).

```bash
# Baixar o dataset principal (A Christmas Carol)
curl -L -o AChristmasCarol_CharlesDickens_English.txt \
  https://raw.githubusercontent.com/FilipeLopesPires/LargeText-WordCount/main/datasets/AChristmasCarol_CharlesDickens/AChristmasCarol_CharlesDickens_English.txt

# Executar (2º argumento = nº de workers; opcional, padrão = nº de CPUs)
go run . AChristmasCarol_CharlesDickens_English.txt
go run . AChristmasCarol_CharlesDickens_English.txt 4

# Ou compilar e rodar o binário
go build -o wordcount .
./wordcount AChristmasCarol_CharlesDickens_English.txt 4
```

### Testes de correção

```bash
go test ./...          # roda os testes de correção
go test -race ./...    # roda os testes com o detector de condição de corrida
go test -bench . -benchmem -run XXX   # mede cada versão isoladamente (benchmark)
```

Os testes validam:
1. a versão **sequencial** contra um resultado conhecido (`sample.txt`);
2. a versão **concorrente** contra o mesmo resultado, para 1/2/4/8 workers;
3. que sequencial e concorrente produzem **mapas idênticos** no dataset principal.

### Exemplo de saída

```
Arquivo: AChristmasCarol_CharlesDickens_English.txt
Workers (concorrente): 4
Palavras distintas: 4307

Tempo sequencial:  5.771199ms
Tempo concorrente: 3.040733ms
Resultados iguais: sim

Top 20 palavras:
 1. the             1629
 2. and             1082
 3. was             435
 4. his             434
 5. scrooge         375
 ...
```

---

## Estratégia concorrente escolhida

**Pool de workers + divisão do texto em blocos + mapas locais + redução final.**

1. O texto é dividido em `N` blocos contíguos (`N` = nº de workers). Os cortes
   acontecem **sempre em espaço em branco**, nunca no meio de uma palavra, e a
   união dos blocos é exatamente o texto original (partição completa, sem
   sobreposição).
2. Cada worker (uma goroutine) conta o seu bloco em um **mapa local próprio**.
   Nenhuma estrutura compartilhada é escrita por mais de uma goroutine.
3. Um `sync.WaitGroup` aguarda todos terminarem.
4. A **redução final** combina (soma) os mapas locais em um único mapa — feita
   já fora das goroutines, de forma sequencial.

O ponto central: **a tokenização/contagem é exatamente a mesma função
(`countInto`) usada pela versão sequencial**. A sequencial chama `countInto`
uma vez sobre o texto inteiro; cada worker chama `countInto` sobre o seu bloco.
Como a partição é completa e a função é idêntica, os dois mapas são iguais por
construção.

### Alternativas consideradas

| Estratégia | Vantagem | Desvantagem / risco |
|---|---|---|
| Mapa global protegido por `sync.Mutex` | Simples de descrever | Alta contenção: todo `counts[w]++` disputa o mesmo lock → costuma ficar **mais lento** que o sequencial |
| `sync.Map` | Evita lock explícito | Otimizado para "muita leitura, pouca escrita"; aqui é escrita intensa → mau desempenho; API mais verbosa |
| Goroutines + channel enviando cada palavra a um agregador | Sem corrida (um só dono do mapa) | O agregador único vira gargalo serial; custo alto de comunicação palavra a palavra |
| **Mapas locais + redução final (escolhida)** | **Sem lock, sem contenção; cada worker roda a 100%** | Custo extra de memória (N mapas) e da fusão final — desprezível aqui |
| `goroutine` por bloco sem pool fixo | Fácil | Para arquivos enormes geraria goroutines demais; pool fixo (= nº de workers) é mais controlado |

### Justificativa

A escolha foi feita por **correção, simplicidade e desempenho**, nessa ordem:

- **Correção:** como não há estado compartilhado mutável, é trivial argumentar
  (e o `-race` confirma) que não existe condição de corrida. Reutilizar a mesma
  função de contagem elimina o risco de as duas versões divergirem por
  diferenças sutis de tokenização.
- **Custo de sincronização:** praticamente nulo durante a fase pesada. A única
  sincronização é o `WaitGroup` (uma barreira) e a redução final, ambos O(N).
  Não há lock no caminho quente, ao contrário do mapa global com mutex.
- **Desempenho:** sem contenção, cada núcleo trabalha de forma independente, o
  que dá o melhor aproveitamento possível de paralelismo para este problema.

---

## Normalização das palavras

Implementada em `countInto`:

- **Separação:** `strings.FieldsFunc` quebra o texto em tokens formados apenas
  por **letras** (`unicode.IsLetter`). Tudo que não é letra (pontuação, dígitos,
  espaços, quebras de linha) funciona como separador — é assim que a "pontuação
  simples" é removida.
- **Minúsculas:** cada token passa por `strings.ToLower`.
- **Tamanho mínimo:** descarta tokens com **menos de 3 runes**. A contagem é
  feita com `utf8.RuneCountInString`, **não** com `len()` (bytes), para tratar
  acentos corretamente — por exemplo, `"é"` tem 1 caractere (e deve ser
  ignorado), mas 2 bytes em UTF-8.

> Limitação conhecida e assumida: contrações como `don't` são quebradas em
> `don` + `t` (e `t` é descartado por ter 1 caractere). Como **as duas versões
> usam a mesma regra**, isso não afeta a comparação de correção.

---

## Comparação entre resultados

A função `sameCounts(a, b)` compara o **mapa completo**, não apenas o Top 20:

1. confere se os dois mapas têm o **mesmo número** de palavras;
2. para cada palavra de `a`, confere se existe em `b` **com a mesma contagem**.

Isso garante "mesmas palavras com as mesmas frequências". O programa imprime
`Resultados iguais: sim/não`. Nos testes automatizados, a mesma verificação é
feita para 1/2/4/8/16 workers.

---

## Medição do tempo

**O que é medido:** apenas a **contagem das palavras** (tokenização +
contagem + redução), usando `time.Now()` / `time.Since()` em volta da chamada da
função. A **leitura do arquivo é feita uma única vez antes** e **não** entra na
medição, para que a comparação seja justa (as duas versões partem do mesmo texto
já em memória).

```go
start := time.Now()
seqCounts := countWordsSequential(text)
seqDur := time.Since(start)
```

### Tempos obtidos

Máquina: Intel Core i7-4770HQ (**4 núcleos físicos / 8 threads**), macOS, Go 1.26.2.

**(a) Medição do programa** (`main`, uma execução, sequencial antes da concorrente):

| Workers | Tempo sequencial | Tempo concorrente |
|--------:|-----------------:|------------------:|
| 1 | ~5,7 ms | ~4,2 ms |
| 2 | ~5,6 ms | ~2,7 ms |
| 4 | ~5,7 ms | ~2,3 ms |
| 8 | ~5,8 ms | ~3,4 ms |

**(b) Benchmark isolado** (`go test -bench`, 200 repetições por versão, "quente"):

| Versão | Tempo/op | vs. sequencial |
|---|---:|---:|
| Sequencial | 3,72 ms | — |
| Concorrente — 1 worker | 4,42 ms | 0,84× (**mais lento**) |
| Concorrente — 2 workers | 2,99 ms | 1,24× |
| Concorrente — 4 workers | **2,75 ms** | **1,35× (melhor)** |
| Concorrente — 8 workers | 4,78 ms | 0,78× (**mais lento**) |

---

## Análise de desempenho

- **Houve ganho, porém modesto (~1,35× com 4 workers).** O arquivo é pequeno
  (~160 KB, ~3–4 ms de trabalho), então o custo fixo de criar goroutines,
  dividir o texto e fazer a redução final pesa bastante em relação ao trabalho
  útil. O paralelismo não tem muito o que amortizar.

- **Com 1 worker a versão concorrente é mais lenta que a sequencial** (4,42 ms
  vs. 3,72 ms no benchmark). Faz sentido: ela faz o *mesmo* trabalho da
  sequencial **mais** o overhead de goroutine + mapa local + fusão, sem nenhum
  paralelismo para compensar. Isso isola e quantifica o custo da estratégia.

- **O ponto ótimo é 4 workers**, exatamente o número de **núcleos físicos**. Com
  8 workers o tempo piora (4,78 ms): são apenas 4 núcleos reais (8 threads via
  hyper-threading), e mais workers significam mais mapas locais para fundir e
  mais disputa por recursos, sem ganho real de paralelismo.

- **Cuidado de medição (viés de ordem).** Na medição do `main`, a versão
  sequencial sempre roda **primeiro** e a concorrente **depois**; a segunda se
  beneficia de cache/alocador já "quentes". Por isso, no `main`, até a
  concorrente com 1 worker parece mais rápida que a sequencial — o que o
  benchmark isolado desmente. **Os números confiáveis para conclusão são os do
  benchmark (b)**, que mede cada versão repetida e separadamente. O `main`
  cumpre o que a atividade pede (medir e comparar), mas a leitura crítica dos
  números exige reconhecer esse viés.

**Conclusão:** para este dataset, concorrência traz ganho real mas limitado;
"usar goroutines" não basta — uma estratégia ruim (mutex global, ou workers
demais) deixaria a versão concorrente **mais lenta** que a sequencial. O ganho
escalaria melhor com arquivos significativamente maiores.

---

## Condições de corrida

- **Onde poderia ocorrer:** se vários workers escrevessem no **mesmo** mapa de
  frequências sem sincronização (o clássico `map` concorrente em Go, que causa
  panic/corrupção), ou se incrementassem um contador compartilhado.
- **Como foi evitada:** cada worker escreve **apenas no seu próprio mapa local**
  e em `locals[i]` com índice exclusivo. Não há nenhuma estrutura mutável
  compartilhada entre goroutines durante a contagem. A fusão dos mapas acontece
  **depois** do `wg.Wait()`, de forma sequencial.
- **A sincronização usada é necessária?** Sim, e é mínima: o `WaitGroup` é
  indispensável para garantir que todos os mapas locais estejam prontos antes da
  redução. Não há `Mutex` porque não há recurso compartilhado a proteger —
  acrescentar um seria contenção desnecessária.
- **Houve contenção?** Não na fase de contagem (cada worker é independente). A
  estratégia foi escolhida justamente para eliminar contenção.
- **Verificação:** `go test -race ./...` passa sem reportar nenhuma corrida.

---

## Principais dificuldades

1. **Acentuação e tamanho mínimo:** contar caracteres por bytes (`len`) trataria
   `"é"` como tendo 2 caracteres. Foi preciso contar **runes**
   (`utf8.RuneCountInString`) para que `"é"`/`"go"`/`"ia"` fossem corretamente
   ignoradas.
2. **Cortar o texto sem partir palavras** nem cair no meio de uma sequência
   UTF-8: resolvido cortando sempre em espaço em branco ASCII.
3. **Garantir mapas idênticos:** resolvido reutilizando a mesma função de
   contagem nas duas versões, em vez de reimplementar a lógica.
4. **Interpretar os tempos:** perceber e documentar o viés de ordem da medição
   foi tão importante quanto medir.

---

## Perguntas finais

- **Ferramenta de IA e ambiente.** Claude (Claude Code / modelo Opus 4.8) como
  apoio. Ambiente: macOS, Go 1.26.2, CPU Intel i7-4770HQ (4 núcleos/8 threads).
- **Estratégia concorrente escolhida.** Pool de workers com divisão do texto em
  blocos, mapas locais por worker e redução final.
- **Alternativas consideradas.** Mutex global, `sync.Map`, channel para um
  agregador único, goroutine por bloco sem pool. (Tabela acima.)
- **Por que a escolhida pareceu mais adequada.** Sem estado compartilhado →
  correção fácil de garantir e sem contenção → melhor desempenho; e simplicidade
  de provar equivalência com a sequencial.
- **O resultado concorrente foi exatamente igual ao sequencial?** Sim —
  `Resultados iguais: sim`, confirmado pela comparação do mapa completo e pelos
  testes para vários números de workers.
- **A versão concorrente melhorou o desempenho?** Sim, mas de forma modesta
  (~1,35× com 4 workers), por causa do tamanho pequeno do arquivo. Com 1 worker
  ou com 8 workers ela fica mais lenta que a sequencial.
- **Onde poderia ocorrer condição de corrida / a sincronização era necessária /
  houve contenção?** Ver seção [Condições de corrida](#condições-de-corrida).
- **A IA sugeriu alguma solução problemática? O que precisou ser corrigido?**
  Ver [`PROMPT.md`](PROMPT.md) — principalmente a tendência inicial de propor
  mapa global com mutex e de medir caracteres por bytes.
- **Diferença entre "usar concorrência" e "construir uma boa solução
  concorrente".** Usar goroutines é fácil e pode até piorar o desempenho (mutex
  global, workers demais, contenção). Uma *boa* solução concorrente exige
  escolher a estratégia certa para o problema — aqui, eliminar o estado
  compartilhado — equilibrando correção, contenção, custo de sincronização e o
  overhead de dividir e recombinar o trabalho.
