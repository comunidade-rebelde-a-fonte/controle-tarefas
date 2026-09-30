package importer

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/unicode/norm"

	"controle/internal/platform/apperr"
)

// Record é uma linha de dados da planilha, com os valores por chave de coluna.
type Record struct {
	Line   int // número da linha na planilha (cabeçalho = 1)
	Values map[string]string
}

// Sheet é o conteúdo lido: linhas não vazias e colunas desconhecidas (ignoradas).
type Sheet struct {
	Records        []Record
	UnknownHeaders []string
}

// normalizeHeader compara cabeçalhos sem acento, caixa, espaços extras e asterisco.
func normalizeHeader(s string) string {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "*"))
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

var headerKeys = func() map[string]string {
	m := make(map[string]string, len(Columns))
	for _, c := range Columns {
		m[normalizeHeader(c.Header)] = c.Key
	}
	return m
}()

func invalidFile(msg string) error { return apperr.BadRequest("INVALID_IMPORT_FILE", msg) }

// Parse lê .xlsx (aba "Empresas" ou a primeira) ou .csv (";" ou ",", UTF-8 ou Windows-1252)
// e mapeia as colunas pelo cabeçalho [PRD §35.2]. Linhas totalmente vazias são ignoradas.
func Parse(name string, data []byte) (Sheet, error) {
	if len(data) == 0 {
		return Sheet{}, invalidFile("O arquivo está vazio.")
	}
	var rows [][]string
	var err error
	switch ext := strings.ToLower(filepath.Ext(name)); {
	case ext == ".xlsx" || bytes.HasPrefix(data, []byte("PK\x03\x04")):
		rows, err = readXLSX(data)
	case ext == ".csv" || ext == ".txt":
		rows, err = readCSV(data)
	default:
		return Sheet{}, invalidFile("Formato não aceito: envie a planilha em .xlsx ou .csv.")
	}
	if err != nil {
		return Sheet{}, err
	}
	return mapRows(rows)
}

func readXLSX(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, invalidFile("Não foi possível ler a planilha .xlsx.")
	}
	defer func() { _ = f.Close() }()
	sheet := SheetCompanies
	if idx, _ := f.GetSheetIndex(sheet); idx < 0 {
		sheet = f.GetSheetName(0)
	}
	rows, err := f.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, invalidFile("Não foi possível ler a aba de empresas da planilha.")
	}
	return rows, nil
}

func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		dec, err := charmap.Windows1252.NewDecoder().Bytes(data)
		if err != nil {
			return nil, invalidFile("Não foi possível ler a codificação do arquivo .csv.")
		}
		data = dec
	}
	firstLine, _, _ := bytes.Cut(data, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ','
	if bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	var rows [][]string
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, invalidFile("Não foi possível ler o arquivo .csv: verifique aspas e separadores.")
		}
		rows = append(rows, rec)
	}
	return rows, nil
}

func mapRows(rows [][]string) (Sheet, error) {
	if len(rows) == 0 {
		return Sheet{}, invalidFile("A planilha não tem cabeçalho.")
	}
	var out Sheet
	keys := make([]string, len(rows[0]))
	seen := map[string]bool{}
	for i, h := range rows[0] {
		if strings.TrimSpace(h) == "" {
			continue
		}
		key, ok := headerKeys[normalizeHeader(h)]
		if !ok {
			out.UnknownHeaders = append(out.UnknownHeaders, strings.TrimSpace(h))
			continue
		}
		if seen[key] {
			return Sheet{}, invalidFile("Coluna repetida no cabeçalho: " + strings.TrimSpace(h) + ".")
		}
		seen[key] = true
		keys[i] = key
	}
	var missing []string
	for _, c := range Columns {
		if c.Required && !seen[c.Key] {
			missing = append(missing, c.Header)
		}
	}
	if len(missing) > 0 {
		return Sheet{}, apperr.BadRequest("INVALID_IMPORT_FILE", "Colunas obrigatórias ausentes no cabeçalho: "+strings.Join(missing, ", ")+". Use o modelo de importação.").
			WithDetails(map[string]any{"missing_headers": missing})
	}
	for i, row := range rows[1:] {
		values := map[string]string{}
		empty := true
		for j, v := range row {
			if j >= len(keys) || keys[j] == "" {
				continue
			}
			v = strings.TrimSpace(v)
			if v != "" {
				empty = false
				values[keys[j]] = v
			}
		}
		if empty {
			continue
		}
		out.Records = append(out.Records, Record{Line: i + 2, Values: values})
	}
	if len(out.Records) == 0 {
		return Sheet{}, invalidFile("A planilha não tem linhas de empresas.")
	}
	return out, nil
}
