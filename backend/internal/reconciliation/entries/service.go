package entries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/reconciliation"
	"controle/internal/store"
	"controle/internal/user"
)

// Service grava e lista os lançamentos do cliente.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// CreateInput é o lançamento do formulário (criar_lancamento da origem, com a empresa obrigatória
// e o documento opcional).
type CreateInput struct {
	CompanyID       string  `json:"company_id"`
	Date            string  `json:"date"`
	Amount          string  `json:"amount"`
	Direction       string  `json:"direction"`
	Counterparty    *string `json:"counterparty"`
	CounterpartyDoc *string `json:"counterparty_doc"`
	DocumentRef     *string `json:"document_ref"`
}

// ImportResult é o resultado da importação do CSV.
type ImportResult struct {
	Inserted int      `json:"inserted"`
	IDs      []string `json:"ids"`
}

func fieldsError(errs fieldErrors) error {
	fields := make(map[string]any, len(errs))
	for k, v := range errs {
		fields[k] = v
	}
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
		WithDetails(map[string]any{"fields": fields})
}

// requireCompany confere a empresa (cadastro canônico).
func requireCompany(ctx context.Context, q *store.Queries, id string) error {
	if !httpx.IsUUID(id) {
		return validate.Field("company_id", "Empresa inválida.")
	}
	if _, err := q.GetCompanyIdentity(ctx, id); errors.Is(err, pgx.ErrNoRows) {
		return validate.Field("company_id", "Empresa não encontrada.")
	} else if err != nil {
		return err
	}
	return nil
}

// requireOpen trava pelo fechamento do Contábil cada competência que os lançamentos tocam.
func requireOpen(ctx context.Context, tx pgx.Tx, companyID string, lines []manualLine) error {
	seen := map[string]competence.Competence{}
	for _, l := range lines {
		c := competence.FromDate(l.date)
		seen[c.String()] = c
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := reconciliation.RequireOpen(ctx, tx, companyID, seen[k]); err != nil {
			return err
		}
	}
	return nil
}

// insert grava os lançamentos manuais e a intervenção humana no log de extração.
func (s *Service) insert(ctx context.Context, tx pgx.Tx, companyID string, lines []manualLine, change map[string]any) ([]string, error) {
	q := store.New(tx)
	uid := user.FromContext(ctx)
	now := s.clock.Now()
	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		amount, date, direction := l.amount, l.date, l.direction
		id, err := q.InsertClientEntry(ctx, store.InsertClientEntryParams{
			CompanyID: companyID, EntryDate: &date, Amount: &amount, Direction: &direction, Counterparty: l.counterparty,
			CounterpartyDoc: l.counterpartyDoc, DocumentRef: l.documentRef, Status: StatusManual, Flags: []byte("[]"),
			CreatedBy: &uid, Now: now,
		})
		if err != nil {
			return nil, fmt.Errorf("gravar o lançamento manual: %w", err)
		}
		ids = append(ids, id)
	}
	change["lancamentos"] = ids
	b, err := json.Marshal(change)
	if err != nil {
		return nil, err
	}
	text := string(b)
	if _, err := q.InsertExtractionLog(ctx, store.InsertExtractionLogParams{
		CompanyID: &companyID, Alias: "humano", Attempt: 0, ActorID: &uid, Change: &text, Now: now,
	}); err != nil {
		return nil, fmt.Errorf("registrar o lançamento manual: %w", err)
	}
	return ids, nil
}

// Create grava o lançamento do formulário.
func (s *Service) Create(ctx context.Context, in CreateInput) (Entry, error) {
	line, errs := in.validate()
	if !httpx.IsUUID(in.CompanyID) {
		errs.add("company_id", "Empresa inválida.")
	}
	if len(errs) > 0 {
		return Entry{}, fieldsError(errs)
	}
	var out Entry
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := store.New(tx)
		if err := requireCompany(ctx, q, in.CompanyID); err != nil {
			return err
		}
		if err := requireOpen(ctx, tx, in.CompanyID, []manualLine{line}); err != nil {
			return err
		}
		ids, err := s.insert(ctx, tx, in.CompanyID, []manualLine{line}, map[string]any{"acao": "inserir_lancamento"})
		if err != nil {
			return err
		}
		row, err := q.GetClientEntry(ctx, ids[0])
		if err != nil {
			return err
		}
		out = FromRow(row)
		return nil
	})
	return out, err
}

// ImportCSV grava os lançamentos do CSV (csv_lancamentos da origem) na empresa: tudo ou nada.
func (s *Service) ImportCSV(ctx context.Context, companyID, fileName string, data []byte) (ImportResult, error) {
	lines, lineErrs := parseCSV(data)
	if len(lineErrs) > 0 {
		shown := lineErrs[:min(len(lineErrs), maxCSVErrors)]
		return ImportResult{}, apperr.BadRequest("INVALID_ENTRIES_CSV",
			fmt.Sprintf("%d linha(s) inválida(s): nada foi importado. Corrija o arquivo e envie de novo.", len(lineErrs))).
			WithDetails(map[string]any{"lines": shown, "invalid_lines": len(lineErrs)})
	}
	if len(lines) == 0 {
		return ImportResult{}, apperr.BadRequest("INVALID_ENTRIES_CSV", "O arquivo não tem lançamentos: use data;valor;sentido;contraparte por linha.")
	}
	var out ImportResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := requireCompany(ctx, store.New(tx), companyID); err != nil {
			return err
		}
		if err := requireOpen(ctx, tx, companyID, lines); err != nil {
			return err
		}
		ids, err := s.insert(ctx, tx, companyID, lines, map[string]any{"acao": "importar_lancamentos_csv", "arquivo": fileName})
		if err != nil {
			return err
		}
		out = ImportResult{Inserted: len(ids), IDs: ids}
		return nil
	})
	return out, err
}

// ListInput são os filtros da lista.
type ListInput struct {
	CompanyID string
	Status    string
	DateFrom  string
	DateTo    string
	Limit     int32
	Offset    int32
}

func optional(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func optionalDate(s, field string) (*time.Time, error) {
	v := optional(s)
	if v == nil {
		return nil, nil
	}
	d, err := time.Parse(time.DateOnly, *v)
	if err != nil {
		return nil, validate.Field(field, "Data inválida: use AAAA-MM-DD.")
	}
	return &d, nil
}

// List lista os lançamentos por data, com os filtros.
func (s *Service) List(ctx context.Context, in ListInput) (EntryList, error) {
	p := store.ListClientEntriesParams{CompanyID: optional(in.CompanyID), Status: optional(in.Status)}
	if p.CompanyID != nil && !httpx.IsUUID(*p.CompanyID) {
		return EntryList{}, validate.Field("company_id", "Empresa inválida.")
	}
	if p.Status != nil && !contains(Statuses, *p.Status) {
		return EntryList{}, validate.Field("status", "Status inválido: use "+strings.Join(Statuses, ", ")+".")
	}
	var err error
	if p.DateFrom, err = optionalDate(in.DateFrom, "date_from"); err != nil {
		return EntryList{}, err
	}
	if p.DateTo, err = optionalDate(in.DateTo, "date_to"); err != nil {
		return EntryList{}, err
	}
	p.RowLimit = in.Limit
	if p.RowLimit == 0 {
		p.RowLimit = 50
	}
	if p.RowLimit < 1 || p.RowLimit > 200 {
		return EntryList{}, validate.Field("limit", "Use um limite entre 1 e 200.")
	}
	if in.Offset < 0 {
		return EntryList{}, validate.Field("offset", "O deslocamento não pode ser negativo.")
	}
	p.RowOffset = in.Offset
	q := store.New(s.pool)
	rows, err := q.ListClientEntries(ctx, p)
	if err != nil {
		return EntryList{}, err
	}
	total, err := q.CountClientEntries(ctx, store.CountClientEntriesParams{CompanyID: p.CompanyID, Status: p.Status, DateFrom: p.DateFrom, DateTo: p.DateTo})
	if err != nil {
		return EntryList{}, err
	}
	out := EntryList{Items: make([]Entry, 0, len(rows)), Total: total}
	for _, r := range rows {
		out.Items = append(out.Items, FromRow(r))
	}
	return out, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// ByDocument é o lançamento lido do comprovante do documento (nil se não houver).
func ByDocument(ctx context.Context, db store.DBTX, documentID string) (*Entry, error) {
	row, err := store.New(db).GetClientEntryByDocument(ctx, &documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e := FromRow(row)
	return &e, nil
}
