package pdftext

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

var shared = New(0)

// O texto do PDF sai byte a byte igual ao do pdfplumber da origem (testdata/text_gen.py).
func TestExtract_MatchesPdfplumber(t *testing.T) {
	pdfs, err := filepath.Glob("testdata/*.pdf")
	if err != nil || len(pdfs) < 6 {
		t.Fatalf("PDFs de teste: %v %v", pdfs, err)
	}
	for _, path := range pdfs {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(strings.TrimSuffix(path, ".pdf") + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			mime, err := DetectMIME(data)
			if err != nil || mime != MimePDF {
				t.Fatalf("DetectMIME = %q, %v", mime, err)
			}
			got, err := shared.Extract(context.Background(), data, mime)
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(want)) == "" {
				if got.Reason != ReasonNoTextLayer || got.Text != "" {
					t.Fatalf("PDF sem camada de texto = %+v", got)
				}
				return
			}
			if got.Reason != "" || got.Text != string(want) {
				t.Fatalf("texto diverge do pdfplumber (motivo %q):\n--- porte\n%s\n--- pdfplumber\n%s", got.Reason, got.Text, want)
			}
		})
	}
}

func TestExtract_CorruptPDF(t *testing.T) {
	got, err := shared.Extract(context.Background(), []byte("%PDF-1.4 lixo sem estrutura"), MimePDF)
	if err != nil {
		t.Fatal(err)
	}
	// Como na origem (test_pdf_corrompido): ilegível ou sem camada de texto.
	if got.Reason != ReasonUnreadable && got.Reason != ReasonNoTextLayer {
		t.Fatalf("PDF corrompido = %+v", got)
	}
}

// Passado o prazo, a extração é interrompida e vira extracao_excedeu_limites; o extrator
// continua usável.
func TestExtract_TimeoutKillsAndRecovers(t *testing.T) {
	data, err := os.ReadFile("testdata/pdf_declaratorio_fator_r_abaixo_28.pdf")
	if err != nil {
		t.Fatal(err)
	}
	e := New(time.Nanosecond)
	t.Cleanup(func() { _ = e.Close() })
	got, err := e.Extract(context.Background(), data, MimePDF)
	if err != nil || got.Reason != ReasonLimits {
		t.Fatalf("extração no prazo de 1 ns = %+v, %v", got, err)
	}
	e.timeout = Timeout
	got, err = e.Extract(context.Background(), data, MimePDF)
	if err != nil || got.Reason != "" || !strings.Contains(got.Text, "Fator r") {
		t.Fatalf("depois do Kill = %+v, %v", got, err)
	}
}

func TestExtract_CanceledContextIsAnError(t *testing.T) {
	data, err := os.ReadFile("testdata/pdf_com_texto.pdf")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := shared.Extract(ctx, data, MimePDF); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado = %v", err)
	}
}

func TestExtract_Concurrent(t *testing.T) {
	data, err := os.ReadFile("testdata/pdf_com_texto.pdf")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := shared.Extract(context.Background(), data, MimePDF)
			if err == nil && got.Text == "" {
				err = errors.New("sem texto")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectMIME_ByContent(t *testing.T) {
	bad := [][]byte{
		[]byte("MZ\x90\x00\x03\x00\x00\x00executavel"),
		[]byte("\x89PNG\r\n\x1a\n\x00\x00imagem"),
		{},
		// latin-1 com menos de 95% imprimíveis
		append([]byte("abc"), 0x81, 0x82, 0x83, 0x84),
	}
	for _, b := range bad {
		if _, err := DetectMIME(b); !errors.Is(err, ErrUnsupported) {
			t.Errorf("DetectMIME(%q) aceitou", b)
		}
	}
	good := map[string]string{
		"%PDF-1.4\n1 0 obj<<>>endobj\ntrailer<<>>\n%%EOF\n":              MimePDF,
		"Extrato do Simples Nacional\nCNPJ Matriz: 11.222.333/0001-81\n": MimeText,
		"Per\xedodo de Apura\xe7\xe3o (PA): 09/2026\r\n":                 MimeText, // latin-1
	}
	for in, want := range good {
		if got, err := DetectMIME([]byte(in)); err != nil || got != want {
			t.Errorf("DetectMIME(%q) = %q, %v", in, got, err)
		}
	}
}

func TestExtract_TextDecodingAndLimit(t *testing.T) {
	got, err := shared.Extract(context.Background(), []byte("Per\xedodo de Apura\xe7\xe3o"), MimeText)
	if err != nil || got.Text != "Período de Apuração" {
		t.Fatalf("latin-1 = %+v, %v", got, err)
	}
	big := strings.Repeat("é", MaxChars+10)
	got, err = shared.Extract(context.Background(), []byte(big), MimeText)
	if err != nil || utf8.RuneCountInString(got.Text) != MaxChars {
		t.Fatalf("texto acima do limite: %d caracteres, %v", utf8.RuneCountInString(got.Text), err)
	}
}
