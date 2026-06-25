package main

import (
	"fmt"
	"os"
	"testing"
)

// resultado esperado para sample.txt, aplicando as regras de processamento
// (minúsculas, remoção de pontuação, ignorar palavras com menos de 3 caracteres).
//
// As palavras "a", "é", "go" e "ia" são ignoradas por terem menos de 3 runes.
var expectedSample = map[string]int{
	"casa":   3,
	"árvore": 2,
	"azul":   1,
	"verde":  1,
	"útil":   1,
	"mas":    1,
	"erra":   1,
}

// TestSequentialSmall valida a versão sequencial contra um resultado conhecido.
func TestSequentialSmall(t *testing.T) {
	text, err := readFile("sample.txt")
	if err != nil {
		t.Fatalf("não foi possível ler sample.txt: %v", err)
	}

	got := countWordsSequential(text)

	if !sameCounts(got, expectedSample) {
		t.Errorf("mapa sequencial diferente do esperado.\nobtido:   %v\nesperado: %v", got, expectedSample)
	}
}

// TestConcurrentSmall valida a versão concorrente contra o mesmo resultado conhecido,
// em várias configurações de workers.
func TestConcurrentSmall(t *testing.T) {
	text, err := readFile("sample.txt")
	if err != nil {
		t.Fatalf("não foi possível ler sample.txt: %v", err)
	}

	for _, w := range []int{1, 2, 4, 8} {
		got := countWordsConcurrent(text, w)
		if !sameCounts(got, expectedSample) {
			t.Errorf("workers=%d: mapa concorrente diferente do esperado.\nobtido:   %v\nesperado: %v", w, got, expectedSample)
		}
	}
}

// TestSeqEqualsConcurrentDataset garante que, no dataset principal, as duas
// versões produzem EXATAMENTE o mesmo mapa, para diferentes números de workers.
func TestSeqEqualsConcurrentDataset(t *testing.T) {
	text := loadDataset(t)

	seq := countWordsSequential(text)
	for _, w := range []int{1, 2, 3, 4, 8, 16} {
		con := countWordsConcurrent(text, w)
		if !sameCounts(seq, con) {
			t.Errorf("workers=%d: versão concorrente difere da sequencial", w)
		}
	}
}

const dataset = "AChristmasCarol_CharlesDickens_English.txt"

func loadDataset(tb testing.TB) string {
	if _, err := os.Stat(dataset); err != nil {
		tb.Skipf("dataset %q não encontrado; pulando", dataset)
	}
	text, err := readFile(dataset)
	if err != nil {
		tb.Fatalf("não foi possível ler o dataset: %v", err)
	}
	return text
}

// BenchmarkSequential mede a contagem sequencial isoladamente (warm, repetida).
func BenchmarkSequential(b *testing.B) {
	text := loadDataset(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		countWordsSequential(text)
	}
}

// BenchmarkConcurrent mede a contagem concorrente isoladamente, para vários
// números de workers. Por rodar cada versão repetidamente e separadamente,
// elimina o viés de cache da medição "sequencial-depois-concorrente" do main.
func BenchmarkConcurrent(b *testing.B) {
	text := loadDataset(b)
	for _, w := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprintf("workers=%d", w), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				countWordsConcurrent(text, w)
			}
		})
	}
}
