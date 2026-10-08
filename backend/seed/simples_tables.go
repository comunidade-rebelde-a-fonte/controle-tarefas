package seed

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/clock"
	"controle/internal/platform/decimal"
)

// simplesTables2018 são as tabelas dos Anexos III e V (LC 123/2006, redação da LC 155/2016,
// vigência desde 2018-01-01), cópia idêntica de fator-r/apps/api/fixtures (TASK-002).
// A VALIDAR (P-04 da origem): conferência das faixas contra o texto legal.
//
//go:embed simples_tables_2018.csv
var simplesTables2018 []byte

// SimplesRow é uma faixa de um anexo numa vigência.
type SimplesRow struct {
	Anexo           string
	Faixa           int16
	RBT12Ate        decimal.Decimal
	AliquotaNominal decimal.Decimal
	ParcelaDeduzir  decimal.Decimal
	VigenciaInicio  time.Time
	VigenciaFim     *time.Time
}

// SimplesTables lê o CSV embutido.
func SimplesTables() ([]SimplesRow, error) {
	records, err := csv.NewReader(bytes.NewReader(simplesTables2018)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("simples_tables_2018.csv vazio")
	}
	out := make([]SimplesRow, 0, len(records)-1)
	for i, r := range records[1:] {
		row, err := parseSimplesRow(r)
		if err != nil {
			return nil, fmt.Errorf("simples_tables_2018.csv, linha %d: %w", i+2, err)
		}
		out = append(out, row)
	}
	return out, nil
}

func parseSimplesRow(r []string) (SimplesRow, error) {
	if len(r) != 7 {
		return SimplesRow{}, fmt.Errorf("esperadas 7 colunas, vieram %d", len(r))
	}
	faixa, err := strconv.ParseInt(r[1], 10, 16)
	if err != nil {
		return SimplesRow{}, fmt.Errorf("faixa: %w", err)
	}
	var row SimplesRow
	row.Anexo, row.Faixa = r[0], int16(faixa)
	for _, f := range []struct {
		dst *decimal.Decimal
		src string
	}{{&row.RBT12Ate, r[2]}, {&row.AliquotaNominal, r[3]}, {&row.ParcelaDeduzir, r[4]}} {
		if *f.dst, err = decimal.Parse(f.src); err != nil {
			return SimplesRow{}, fmt.Errorf("valor %q: %w", f.src, err)
		}
	}
	if row.VigenciaInicio, err = time.Parse(time.DateOnly, r[5]); err != nil {
		return SimplesRow{}, fmt.Errorf("vigencia_inicio: %w", err)
	}
	if r[6] != "" {
		fim, err := time.Parse(time.DateOnly, r[6])
		if err != nil {
			return SimplesRow{}, fmt.Errorf("vigencia_fim: %w", err)
		}
		row.VigenciaFim = &fim
	}
	return row, nil
}

// simplesTablesStep carrega as faixas. Como na origem (carregar_csv), insere só as linhas
// novas e nunca altera uma vigência já gravada.
func simplesTablesStep() Step {
	return Step{Name: "simples_tables_2018.csv (Fator R)", Run: func(ctx context.Context, tx pgx.Tx, _ clock.Clock) error {
		rows, err := SimplesTables()
		if err != nil {
			return err
		}
		batch := &pgx.Batch{}
		for _, r := range rows {
			batch.Queue(`INSERT INTO factor_r_simples_tables
			                 (anexo, faixa, rbt12_ate, aliquota_nominal, parcela_deduzir, vigencia_inicio, vigencia_fim)
			             VALUES ($1, $2, $3, $4, $5, $6, $7)
			             ON CONFLICT (anexo, faixa, vigencia_inicio) DO NOTHING`,
				r.Anexo, r.Faixa, r.RBT12Ate, r.AliquotaNominal, r.ParcelaDeduzir, r.VigenciaInicio, r.VigenciaFim)
		}
		return tx.SendBatch(ctx, batch).Close()
	}}
}
