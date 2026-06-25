// Programa de contagem de frequência de palavras em um arquivo texto.
//
// Possui duas versões da contagem:
//   - sequencial  (countWordsSequential)  -> usada como referência de correção
//   - concorrente (countWordsConcurrent)  -> pool de workers + mapas locais + redução final
//
// O programa lê o arquivo, executa as duas versões, mede o tempo de cada uma,
// compara os mapas COMPLETOS de frequência e imprime as 20 palavras mais frequentes.
//
// Uso:
//   go run . <arquivo.txt> [num_workers]
// Exemplo:
//   go run . AChristmasCarol_CharlesDickens_English.txt 4
package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// minWordLen: palavras com MENOS de 3 caracteres (runes) são ignoradas.
const minWordLen = 3

// readFile lê todo o conteúdo do arquivo e o devolve como string.
func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// normalizeWord aplica a normalização de uma palavra já isolada:
// converte para minúsculas. A remoção de pontuação acontece na tokenização
// (countInto), que separa o texto apenas por sequências de letras.
func normalizeWord(word string) string {
	return strings.ToLower(word)
}

// countInto tokeniza `text` e acumula as frequências em `counts`.
//
// É o CORAÇÃO compartilhado pelas duas versões: tanto a sequencial quanto cada
// worker da concorrente chamam exatamente esta mesma função. Isso garante, por
// construção, que as regras de processamento sejam idênticas e que os dois
// mapas resultantes sejam iguais.
//
// Regras aplicadas aqui:
//   - separa o texto em tokens formados apenas por letras (remove pontuação,
//     dígitos e espaços) usando strings.FieldsFunc + unicode.IsLetter;
//   - converte cada token para minúsculas;
//   - ignora tokens com menos de minWordLen caracteres (contagem por runes,
//     não por bytes, para tratar acentos corretamente: "é" tem 1 caractere).
func countInto(counts map[string]int, text string) {
	tokens := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r)
	})
	for _, tok := range tokens {
		w := normalizeWord(tok)
		if utf8.RuneCountInString(w) >= minWordLen {
			counts[w]++
		}
	}
}

// countWordsSequential conta as palavras sem nenhum recurso de concorrência.
// Serve de referência para validar a versão concorrente.
func countWordsSequential(text string) map[string]int {
	counts := make(map[string]int)
	countInto(counts, text)
	return counts
}

// countWordsConcurrent conta as palavras usando um pool de `workers` goroutines.
//
// Estratégia: o texto é dividido em `workers` blocos (sempre cortados em
// espaço em branco, nunca no meio de uma palavra). Cada worker conta o seu
// bloco em um MAPA LOCAL próprio — não há estrutura compartilhada escrita por
// mais de uma goroutine, portanto não há condição de corrida e não é preciso
// mutex. Ao final, os mapas locais são combinados (redução) em um único mapa.
func countWordsConcurrent(text string, workers int) map[string]int {
	if workers < 1 {
		workers = 1
	}

	chunks := splitIntoChunks(text, workers)

	// Cada goroutine escreve apenas em locals[i] (índice exclusivo) -> sem corrida.
	locals := make([]map[string]int, len(chunks))
	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk string) {
			defer wg.Done()
			local := make(map[string]int)
			countInto(local, chunk)
			locals[i] = local
		}(i, chunk)
	}
	wg.Wait()

	// Redução final: combina os mapas locais em um único mapa (sequencial,
	// já fora das goroutines, então também é livre de corrida).
	result := make(map[string]int)
	for _, local := range locals {
		mergeCounts(result, local)
	}
	return result
}

// splitIntoChunks divide o texto em até n pedaços contíguos, cortando sempre
// em um espaço em branco ASCII para nunca partir uma palavra ao meio.
// A união dos pedaços é exatamente o texto original (partição completa, sem
// sobreposição), o que garante que cada palavra seja contada uma única vez.
func splitIntoChunks(text string, n int) []string {
	if n <= 1 || len(text) == 0 {
		return []string{text}
	}

	chunkSize := len(text) / n
	if chunkSize == 0 {
		// Texto menor que o número de workers: não vale a pena dividir.
		return []string{text}
	}

	chunks := make([]string, 0, n)
	start := 0
	for i := 0; i < n-1 && start < len(text); i++ {
		end := start + chunkSize
		if end > len(text) {
			end = len(text)
		}
		// Avança o fim até o próximo espaço em branco para não cortar palavra.
		// Espaços são bytes ASCII (< 0x80), então isto nunca cai no meio de
		// uma sequência UTF-8 multibyte.
		for end < len(text) && !isASCIISpace(text[end]) {
			end++
		}
		chunks = append(chunks, text[start:end])
		start = end
	}
	if start < len(text) {
		chunks = append(chunks, text[start:])
	}
	return chunks
}

func isASCIISpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\v' || b == '\f'
}

// mergeCounts soma as frequências de src em dst.
func mergeCounts(dst, src map[string]int) {
	for word, c := range src {
		dst[word] += c
	}
}

// sameCounts verifica se dois mapas de frequência são EXATAMENTE iguais:
// mesmas palavras e mesma contagem para cada palavra. Compara o mapa completo,
// não apenas as palavras mais frequentes.
func sameCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for word, ca := range a {
		if cb, ok := b[word]; !ok || ca != cb {
			return false
		}
	}
	return true
}

// wordCount é um par (palavra, contagem) usado para ordenar o ranking.
type wordCount struct {
	word  string
	count int
}

// topN devolve as n palavras mais frequentes, ordenadas por contagem
// decrescente. Em caso de empate na contagem, ordena alfabeticamente para
// que a saída seja determinística.
func topN(counts map[string]int, n int) []wordCount {
	list := make([]wordCount, 0, len(counts))
	for word, c := range counts {
		list = append(list, wordCount{word: word, count: c})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].word < list[j].word
	})
	if n > len(list) {
		n = len(list)
	}
	return list[:n]
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: go run . <arquivo.txt> [num_workers]")
		os.Exit(1)
	}
	path := os.Args[1]

	// Número de workers: 2º argumento opcional; por padrão usa o número de CPUs.
	workers := runtime.NumCPU()
	if len(os.Args) >= 3 {
		w, err := strconv.Atoi(os.Args[2])
		if err != nil || w < 1 {
			fmt.Fprintf(os.Stderr, "num_workers inválido: %q (use um inteiro >= 1)\n", os.Args[2])
			os.Exit(1)
		}
		workers = w
	}

	text, err := readFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao ler o arquivo %q: %v\n", path, err)
		os.Exit(1)
	}

	// --- Medição da versão sequencial (apenas a contagem) ---
	startSeq := time.Now()
	seqCounts := countWordsSequential(text)
	seqDur := time.Since(startSeq)

	// --- Medição da versão concorrente (apenas a contagem) ---
	startCon := time.Now()
	conCounts := countWordsConcurrent(text, workers)
	conDur := time.Since(startCon)

	// --- Comparação de correção (mapa COMPLETO) ---
	iguais := sameCounts(seqCounts, conCounts)
	resp := "não"
	if iguais {
		resp = "sim"
	}

	fmt.Printf("Arquivo: %s\n", path)
	fmt.Printf("Workers (concorrente): %d\n", workers)
	fmt.Printf("Palavras distintas: %d\n", len(seqCounts))
	fmt.Println()
	fmt.Printf("Tempo sequencial:  %v\n", seqDur)
	fmt.Printf("Tempo concorrente: %v\n", conDur)
	fmt.Printf("Resultados iguais: %s\n", resp)
	fmt.Println()
	fmt.Println("Top 20 palavras:")
	for i, wc := range topN(seqCounts, 20) {
		fmt.Printf("%2d. %-15s %d\n", i+1, wc.word, wc.count)
	}
}
