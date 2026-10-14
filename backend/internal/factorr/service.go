// Package factorr é o módulo Fator R [REQ-008 a REQ-011, TASK-011]: configuração por empresa,
// série mensal de receita e folha, cálculo do PA e carteira do escritório. As contas são do
// motor (subpacote engine); aqui ficam leitura, validação e gravação. A empresa é sempre a do
// cadastro canônico (companies), e "sujeita ao Fator R" é o companies.factor_r (D-017).
//
// O Fator R nunca altera obrigações [REQ-016, AC-011]: este pacote não escreve em nenhuma
// tabela do Controle de Obrigações.
package factorr

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	co "controle/internal/company"
	"controle/internal/factorr/engine"
	"controle/internal/platform/apperr"
	"controle/internal/platform/clock"
	"controle/internal/platform/competence"
	"controle/internal/platform/decimal"
	"controle/internal/platform/opt"
	"controle/internal/store"
	"controle/internal/user"
)

const (
	companyNotFound = "COMPANY_NOT_FOUND"
	companyMissing  = "Empresa não encontrada."
	simplesNational = "SIMPLES_NACIONAL"
)

// Service contém as regras do módulo Fator R.
type Service struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

// NewService cria o serviço.
func NewService(pool *pgxpool.Pool, clk clock.Clock) *Service {
	return &Service{pool: pool, clock: clk}
}

// CurrentPA é o PA padrão: a competência corrente, como na origem (competencia_corrente).
func (s *Service) CurrentPA() competence.Competence { return competence.Current(s.clock) }

// read executa fn numa transação somente leitura com uma fotografia única do banco: cálculo e
// carteira leem empresa, movimentos, tabelas e política do mesmo instante.
func (s *Service) read(ctx context.Context, fn func(q *store.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return fmt.Errorf("iniciar leitura: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(store.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Escalas das colunas. O pgx devolve o zero de qualquer escala como 0 (decimal.ScanNumeric);
// o motor precisa da escala da coluna (0.00, 0.300000) para dar o mesmo resultado da origem,
// que lia os valores do banco com a escala. Como a coluna já tem essa escala, não há
// arredondamento de fato.
func money(d decimal.Decimal) decimal.Decimal { return d.Quantize(-2) }
func rate(d decimal.Decimal) decimal.Decimal  { return d.Quantize(-6) }

func moneyPtr(d *decimal.Decimal) *decimal.Decimal {
	if d == nil {
		return nil
	}
	v := money(*d)
	return &v
}

func company(ctx context.Context, q *store.Queries, id string, ref competence.Competence) (store.GetFactorRCompanyRow, error) {
	c, err := q.GetFactorRCompany(ctx, store.GetFactorRCompanyParams{Ref: ref.FirstDay(), ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return c, apperr.NotFound(companyNotFound, companyMissing)
	}
	return c, err
}

// --- Configuração ---

func defaultSettings(companyID string) Settings {
	return Settings{CompanyID: companyID, QuantidadeSocios: 1, HonorarioMensal: decimal.MustParse("0.00")}
}

func toSettings(r store.CompanyFactorRSetting) Settings {
	updated := r.UpdatedAt
	out := Settings{
		CompanyID: r.CompanyID, Cnae: r.Cnae, Atividade: r.Atividade, QuantidadeSocios: r.QuantidadeSocios,
		Contato: r.Contato, PacoteComercial: r.PacoteComercial, HonorarioMensal: money(r.HonorarioMensal),
		Notas: r.Notas, Version: r.Version, UpdatedAt: &updated,
	}
	if r.InicioAtividade != nil {
		c := competence.FromDate(*r.InicioAtividade)
		out.InicioAtividade = &c
	}
	return out
}

func settingsOf(ctx context.Context, q *store.Queries, companyID string) (Settings, bool, error) {
	r, err := q.GetFactorRSettings(ctx, companyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultSettings(companyID), false, nil
	}
	if err != nil {
		return Settings{}, false, err
	}
	return toSettings(r), true, nil
}

// Settings devolve a configuração do Fator R da empresa (padrões e version 0 sem configuração).
func (s *Service) Settings(ctx context.Context, companyID string) (Settings, error) {
	var out Settings
	err := s.read(ctx, func(q *store.Queries) error {
		if _, err := company(ctx, q, companyID, s.CurrentPA()); err != nil {
			return err
		}
		var err error
		out, _, err = settingsOf(ctx, q, companyID)
		return err
	})
	return out, err
}

// SettingsInput é o payload de PATCH /companies/{id}/factor-r/settings: só os campos enviados
// mudam. version 0 cria a configuração; divergência de versão → 409.
type SettingsInput struct {
	Version          int32                `json:"version"`
	Cnae             opt.Optional[string] `json:"cnae"`
	Atividade        opt.Optional[string] `json:"atividade"`
	QuantidadeSocios *int32               `json:"quantidade_socios"`
	Contato          opt.Optional[string] `json:"contato"`
	PacoteComercial  opt.Optional[string] `json:"pacote_comercial"`
	HonorarioMensal  *string              `json:"honorario_mensal"`
	InicioAtividade  opt.Optional[string] `json:"inicio_atividade"`
	Notas            opt.Optional[string] `json:"notas"`
}

// fieldErrors junta os erros de validação no envelope padrão (400 VALIDATION_ERROR).
type fieldErrors map[string]any

func (f fieldErrors) err() error {
	if len(f) == 0 {
		return nil
	}
	return apperr.BadRequest("VALIDATION_ERROR", "Dados inválidos. Verifique os campos destacados.").
		WithDetails(map[string]any{"fields": map[string]any(f)})
}

// text limpa um texto opcional (vazio = null) e confere o tamanho em caracteres.
func (f fieldErrors) text(field string, v *string, maxLen int) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	if utf8.RuneCountInString(t) > maxLen {
		f[field] = fmt.Sprintf("Máximo de %d caracteres.", maxLen)
	}
	return &t
}

// money lê um valor em reais: string decimal não negativa com até 12 dígitos inteiros e 2
// casas (Numeric(14,2) da origem). Ausente vale 0,00, como na origem.
func (f fieldErrors) money(field string, v *string) decimal.Decimal {
	if v == nil {
		return decimal.MustParse("0.00")
	}
	d, err := decimal.Parse(strings.TrimSpace(*v))
	switch {
	case err != nil || !d.Fits(12, 2):
		f[field] = "Informe um valor com até 12 dígitos inteiros e 2 casas decimais, como 1234.56."
		return decimal.Zero
	case d.Sign() < 0:
		f[field] = "O valor não pode ser negativo."
		return decimal.Zero
	}
	return money(d)
}

// UpdateSettings grava a configuração do Fator R da empresa.
func (s *Service) UpdateSettings(ctx context.Context, companyID string, in SettingsInput) (Settings, error) {
	fields := fieldErrors{}
	if in.Version < 0 {
		fields["version"] = "Valor mínimo: 0."
	}
	q := store.New(s.pool)
	if _, err := company(ctx, q, companyID, s.CurrentPA()); err != nil {
		return Settings{}, err
	}
	cur, exists, err := settingsOf(ctx, q, companyID)
	if err != nil {
		return Settings{}, err
	}
	next := cur
	if in.Cnae.Set {
		next.Cnae = fields.text("cnae", in.Cnae.Value, 20)
	}
	if in.Atividade.Set {
		next.Atividade = fields.text("atividade", in.Atividade.Value, 500)
	}
	if in.QuantidadeSocios != nil {
		if *in.QuantidadeSocios < 0 || *in.QuantidadeSocios > 1000 {
			fields["quantidade_socios"] = "Informe um número de 0 a 1000."
		}
		next.QuantidadeSocios = *in.QuantidadeSocios
	}
	if in.Contato.Set {
		next.Contato = fields.text("contato", in.Contato.Value, 500)
	}
	if in.PacoteComercial.Set {
		next.PacoteComercial = in.PacoteComercial.Value
		if p := in.PacoteComercial.Value; p != nil && !slices.Contains(packages, *p) {
			fields["pacote_comercial"] = "Valor deve ser um de: monitoramento, correcao, retainer."
		}
	}
	if in.HonorarioMensal != nil {
		next.HonorarioMensal = fields.money("honorario_mensal", in.HonorarioMensal)
	}
	if in.InicioAtividade.Set {
		next.InicioAtividade = nil
		if v := in.InicioAtividade.Value; v != nil && strings.TrimSpace(*v) != "" {
			c, err := competence.Parse(strings.TrimSpace(*v))
			if err != nil {
				fields["inicio_atividade"] = "Competência inválida (use AAAA-MM)."
			}
			next.InicioAtividade = &c
		}
	}
	if in.Notas.Set {
		next.Notas = fields.text("notas", in.Notas.Value, 5000)
	}
	if err := fields.err(); err != nil {
		return Settings{}, err
	}

	var start *time.Time
	if next.InicioAtividade != nil {
		t := next.InicioAtividade.FirstDay()
		start = &t
	}
	uid := user.FromContext(ctx)
	now := s.clock.Now()
	var row store.CompanyFactorRSetting
	if in.Version == 0 {
		if exists {
			return Settings{}, apperr.VersionConflict().WithDetails(map[string]any{"current_version": cur.Version})
		}
		row, err = q.InsertFactorRSettings(ctx, store.InsertFactorRSettingsParams{
			CompanyID: companyID, Cnae: next.Cnae, Atividade: next.Atividade, QuantidadeSocios: next.QuantidadeSocios,
			Contato: next.Contato, PacoteComercial: next.PacoteComercial, HonorarioMensal: next.HonorarioMensal,
			InicioAtividade: start, Notas: next.Notas, UpdatedBy: &uid, UpdatedAt: now,
		})
	} else {
		row, err = q.UpdateFactorRSettings(ctx, store.UpdateFactorRSettingsParams{
			CompanyID: companyID, Version: in.Version, Cnae: next.Cnae, Atividade: next.Atividade,
			QuantidadeSocios: next.QuantidadeSocios, Contato: next.Contato, PacoteComercial: next.PacoteComercial,
			HonorarioMensal: next.HonorarioMensal, InicioAtividade: start, Notas: next.Notas,
			UpdatedBy: &uid, UpdatedAt: now,
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		details := map[string]any{}
		if exists {
			details["current_version"] = cur.Version
		}
		return Settings{}, apperr.VersionConflict().WithDetails(details)
	}
	if err != nil {
		return Settings{}, err
	}
	return toSettings(row), nil
}

// --- Movimentos ---

func toMovement(r store.FactorRMovement) Movement {
	m := Movement{
		Competence:   competence.FromDate(r.Competence),
		ReceitaBruta: money(r.ReceitaBruta), ProLabore: money(r.ProLabore),
		Salarios13Ferias: money(r.Salarios13Ferias), CppRecolhida: money(r.CppRecolhida),
		FgtsRecolhido:            money(r.FgtsRecolhido),
		FolhaTotalDeclaradaPgdas: moneyPtr(r.FolhaTotalDeclaradaPgdas),
		FolhaMes:                 decimal.MustParse("0.00"),
		Origem:                   r.Origem, Observacao: r.Observacao, PgdasDocumentID: r.PgdasDocumentID,
		Version: r.Version, UpdatedAt: r.UpdatedAt,
	}
	if r.FolhaMes != nil {
		m.FolhaMes = money(*r.FolhaMes)
	}
	return m
}

func toEngineMovement(r store.FactorRMovement) engine.Movement {
	return engine.Movement{
		Competence:   competence.FromDate(r.Competence),
		GrossRevenue: money(r.ReceitaBruta), ProLabore: money(r.ProLabore),
		Salaries: money(r.Salarios13Ferias), CPP: money(r.CppRecolhida), FGTS: money(r.FgtsRecolhido),
		DeclaredPayroll: moneyPtr(r.FolhaTotalDeclaradaPgdas), Origin: r.Origem,
	}
}

// MovementsRange limita a listagem; sem limites, todos os meses da empresa.
type MovementsRange struct {
	From, To *competence.Competence
}

// Movements lista os movimentos da empresa em ordem de competência.
func (s *Service) Movements(ctx context.Context, companyID string, rg MovementsRange) (MovementList, error) {
	from, to := competence.New(1, 1), competence.New(9999, 12)
	if rg.From != nil {
		from = *rg.From
	}
	if rg.To != nil {
		to = *rg.To
	}
	out := MovementList{Items: []Movement{}}
	err := s.read(ctx, func(q *store.Queries) error {
		if _, err := company(ctx, q, companyID, s.CurrentPA()); err != nil {
			return err
		}
		rows, err := q.ListFactorRMovements(ctx, store.ListFactorRMovementsParams{
			CompanyID: companyID, FromCompetence: from.FirstDay(), ToCompetence: to.FirstDay(),
		})
		for _, r := range rows {
			out.Items = append(out.Items, toMovement(r))
		}
		return err
	})
	return out, err
}

// MovementInput é o payload de PUT /companies/{id}/factor-r/movements/{competence}. Valor
// ausente vale 0,00. version 0 cria o mês; divergência de versão → 409.
type MovementInput struct {
	Version          int32   `json:"version"`
	ReceitaBruta     *string `json:"receita_bruta"`
	ProLabore        *string `json:"pro_labore"`
	Salarios13Ferias *string `json:"salarios_13_ferias"`
	CppRecolhida     *string `json:"cpp_recolhida"`
	FgtsRecolhido    *string `json:"fgts_recolhido"`
	Observacao       *string `json:"observacao"`
}

// PutMovement lança o mês à mão: cria a competência ou a substitui inteira, com origem manual,
// como na origem (upsert_manual). Um mês do PGDAS-D detalhado assim deixa de ter a folha total
// declarada e passa a ter a divisão informada.
func (s *Service) PutMovement(ctx context.Context, companyID string, c competence.Competence, in MovementInput) (Movement, error) {
	fields := fieldErrors{}
	if in.Version < 0 {
		fields["version"] = "Valor mínimo: 0."
	}
	revenue := fields.money("receita_bruta", in.ReceitaBruta)
	proLabore := fields.money("pro_labore", in.ProLabore)
	salaries := fields.money("salarios_13_ferias", in.Salarios13Ferias)
	cpp := fields.money("cpp_recolhida", in.CppRecolhida)
	fgts := fields.money("fgts_recolhido", in.FgtsRecolhido)
	note := fields.text("observacao", in.Observacao, 1000)
	if err := fields.err(); err != nil {
		return Movement{}, err
	}

	q := store.New(s.pool)
	if _, err := company(ctx, q, companyID, s.CurrentPA()); err != nil {
		return Movement{}, err
	}
	uid := user.FromContext(ctx)
	now := s.clock.Now()
	var (
		row store.FactorRMovement
		err error
	)
	if in.Version == 0 {
		row, err = q.InsertFactorRMovementManual(ctx, store.InsertFactorRMovementManualParams{
			CompanyID: companyID, Competence: c.FirstDay(), ReceitaBruta: revenue, ProLabore: proLabore,
			Salarios13Ferias: salaries, CppRecolhida: cpp, FgtsRecolhido: fgts, Observacao: note,
			UpdatedBy: &uid, UpdatedAt: now,
		})
	} else {
		row, err = q.UpdateFactorRMovementManual(ctx, store.UpdateFactorRMovementManualParams{
			CompanyID: companyID, Competence: c.FirstDay(), Version: in.Version, ReceitaBruta: revenue,
			ProLabore: proLabore, Salarios13Ferias: salaries, CppRecolhida: cpp, FgtsRecolhido: fgts,
			Observacao: note, UpdatedBy: &uid, UpdatedAt: now,
		})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		details := map[string]any{}
		if cur, err := q.GetFactorRMovement(ctx, store.GetFactorRMovementParams{CompanyID: companyID, Competence: c.FirstDay()}); err == nil {
			details["current_version"] = cur.Version
		}
		return Movement{}, apperr.VersionConflict().WithDetails(details)
	}
	if err != nil {
		return Movement{}, err
	}
	return toMovement(row), nil
}

// --- Cálculo e carteira ---

func tableNotInForce(pa competence.Competence) error {
	return apperr.Unprocessable("SIMPLES_TABLE_NOT_IN_FORCE",
		fmt.Sprintf("Não há tabela do Simples vigente para o PA %s.", pa.Display()))
}

// tables monta os Anexos III e V vigentes no PA, cada um com as faixas da vigência mais
// recente (tabelas_vigentes). Sem tabela vigente → 422.
func tables(ctx context.Context, q *store.Queries, pa competence.Competence) (engine.Tables, error) {
	rows, err := q.ListSimplesTablesInForce(ctx, pa.FirstDay())
	if err != nil {
		return engine.Tables{}, err
	}
	build := func(a engine.Annex) (engine.AnnexTable, error) {
		t := engine.AnnexTable{Annex: a}
		found := false
		for _, r := range rows {
			if r.Anexo != string(a) {
				continue
			}
			if !found {
				t.ValidFrom, found = r.VigenciaInicio, true
			}
			if r.VigenciaInicio.Equal(t.ValidFrom) {
				t.Brackets = append(t.Brackets, engine.Bracket{
					Number: int(r.Faixa), RBT12UpTo: money(r.Rbt12Ate),
					NominalRate: rate(r.AliquotaNominal), Deduction: money(r.ParcelaDeduzir),
				})
			}
		}
		if !found {
			return t, tableNotInForce(pa)
		}
		return t, nil
	}
	iii, err := build(engine.AnnexIII)
	if err != nil {
		return engine.Tables{}, err
	}
	v, err := build(engine.AnnexV)
	if err != nil {
		return engine.Tables{}, err
	}
	return engine.Tables{III: iii, V: v}, nil
}

func policy(ctx context.Context, q *store.Queries) (engine.Policy, error) {
	p, err := q.GetFactorRPolicy(ctx)
	if err != nil {
		return engine.Policy{}, fmt.Errorf("política do Fator R: %w", err)
	}
	return engine.Policy{CPPInFS12: p.CppDasIntegraFs12, Target: rate(p.MetaOperacional)}, nil
}

func monitored(active bool, factorR *bool, regime string) bool {
	return active && factorR != nil && *factorR && regime == simplesNational
}

// Calc calcula o Fator R da empresa no PA [AC-005, AC-006].
func (s *Service) Calc(ctx context.Context, companyID string, pa competence.Competence) (Calc, error) {
	var out Calc
	err := s.read(ctx, func(q *store.Queries) error {
		var err error
		out, err = CalcWith(ctx, q, companyID, pa)
		return err
	})
	return out, err
}

// CalcWith calcula o Fator R da empresa no PA com as consultas dadas, para quem já está numa
// transação (o simulador e os agentes, na transação da corrida). Empresa inexistente: 404;
// sem tabela do Simples vigente no PA: 422.
func CalcWith(ctx context.Context, q *store.Queries, companyID string, pa competence.Competence) (Calc, error) {
	co, err := company(ctx, q, companyID, pa)
	if err != nil {
		return Calc{}, err
	}
	set, _, err := settingsOf(ctx, q, companyID)
	if err != nil {
		return Calc{}, err
	}
	w := engine.NewWindow(pa)
	rows, err := q.ListFactorRMovements(ctx, store.ListFactorRMovementsParams{
		CompanyID: companyID, FromCompetence: w.Start.FirstDay(), ToCompetence: w.End.FirstDay(),
	})
	if err != nil {
		return Calc{}, err
	}
	tb, err := tables(ctx, q, pa)
	if err != nil {
		return Calc{}, err
	}
	pol, err := policy(ctx, q)
	if err != nil {
		return Calc{}, err
	}
	ms := make([]engine.Movement, len(rows))
	for i, r := range rows {
		ms[i] = toEngineMovement(r)
	}
	out := Calc{Result: engine.Calculate(pa, set.InicioAtividade, ms, tb, pol)}
	if co.TaxRegime != "" {
		out.RegimeNoPA = &co.TaxRegime
	}
	out.Monitorada = monitored(co.Active, co.FactorR, co.TaxRegime)
	return out, nil
}

// Portfolio monta a carteira do PA: empresas ativas, sujeitas ao Fator R e no Simples Nacional
// no PA, ordenadas por semáforo e economia [REQ-011, AC-007]. Quatro consultas fixas,
// independentemente do número de empresas. Os filtros de empresa do Dashboard valem pelo snapshot
// da competência do PA, com o responsável do Fiscal (TASK-024, D-038; decisão do usuário).
func (s *Service) Portfolio(ctx context.Context, pa competence.Competence, f co.Filter) (Portfolio, error) {
	var out Portfolio
	err := s.read(ctx, func(q *store.Queries) error {
		ids, err := f.CompanyIDs(ctx, q, pa, co.ModuleFiscal)
		if err != nil {
			return err
		}
		out, err = PortfolioWith(ctx, q, pa, ids)
		return err
	})
	return out, err
}

// PortfolioWith monta a carteira do PA com as consultas dadas (o priorizador, na transação da
// corrida), só com as empresas dadas (nil: todas).
func PortfolioWith(ctx context.Context, q *store.Queries, pa competence.Competence, companyIDs []string) (Portfolio, error) {
	var out Portfolio
	err := func() error {
		all, err := q.ListFactorRPortfolioCompanies(ctx, pa.FirstDay())
		if err != nil {
			return err
		}
		rows := all
		if companyIDs != nil {
			keep := make(map[string]bool, len(companyIDs))
			for _, id := range companyIDs {
				keep[id] = true
			}
			rows = rows[:0:0]
			for _, r := range all {
				if keep[r.ID] {
					rows = append(rows, r)
				}
			}
		}
		ids := make([]string, len(rows))
		companies := make([]engine.PortfolioCompany, len(rows))
		refs := make(map[string]store.ListFactorRPortfolioCompaniesRow, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
			refs[r.ID] = r
			c := engine.PortfolioCompany{ID: r.ID, Name: r.LegalName, Package: r.PacoteComercial, MonthlyFee: money(r.HonorarioMensal)}
			if r.InicioAtividade != nil {
				start := competence.FromDate(*r.InicioAtividade)
				c.ActivityStart = &start
			}
			companies[i] = c
		}
		w := engine.NewWindow(pa)
		movs, err := q.ListFactorRMovementsForCompanies(ctx, store.ListFactorRMovementsForCompaniesParams{
			CompanyIds: ids, FromCompetence: w.Start.FirstDay(), ToCompetence: w.End.FirstDay(),
		})
		if err != nil {
			return err
		}
		byCompany := make(map[string][]engine.Movement, len(rows))
		for _, m := range movs {
			byCompany[m.CompanyID] = append(byCompany[m.CompanyID], toEngineMovement(m))
		}
		tb, err := tables(ctx, q, pa)
		if err != nil {
			return err
		}
		pol, err := policy(ctx, q)
		if err != nil {
			return err
		}
		p := engine.BuildPortfolio(pa, companies, byCompany, tb, pol)
		out = Portfolio{PA: pa, MetaOperacional: pol.Target, KPIs: p.KPIs, Linhas: make([]PortfolioLine, len(p.Lines))}
		for i, l := range p.Lines {
			ref, r := refs[l.Company.ID], l.Result
			out.Linhas[i] = PortfolioLine{
				Company:         CompanyRef{ID: ref.ID, InternalCode: ref.InternalCode, LegalName: ref.LegalName, TradeName: ref.TradeName, Cnpj: ref.Cnpj},
				PacoteComercial: l.Company.Package, HonorarioMensal: l.Company.MonthlyFee,
				Acao: l.Action, AcaoTexto: engine.ActionTexts[l.Action],
				Status: r.Status, Motivo: r.Reason, Semaforo: r.Light, FatorR: r.FactorR, Anexo: r.Annex,
				RBT12: r.RBT12, FS12: r.FS12, ReforcoMensal28: r.MonthlyTopUp28, ReforcoMensalMeta: r.MonthlyTopUpTarget,
				MetaOperacional: r.Target, Economia12m: r.Savings12m, MesesFaltantes: r.MissingMonths,
			}
		}
		return nil
	}()
	return out, err
}

// Policy devolve a política do escritório: meta operacional, CPP na FS12, piso de economia,
// limiar do parser e tolerância da nota ouro.
func Policy(ctx context.Context, q *store.Queries) (store.FactorRPolicy, error) {
	p, err := q.GetFactorRPolicy(ctx)
	if err != nil {
		return store.FactorRPolicy{}, fmt.Errorf("política do Fator R: %w", err)
	}
	p.MetaOperacional = rate(p.MetaOperacional)
	p.PisoEconomiaAnual = money(p.PisoEconomiaAnual)
	return p, nil
}
