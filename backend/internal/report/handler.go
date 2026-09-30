package report

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/company"
	"controle/internal/credential"
	"controle/internal/dashboard"
	"controle/internal/grid"
	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/clock"
	comp "controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
)

// Handler expõe GET /reports/{report}.pdf [PRD §36.3, §41.13].
type Handler struct {
	q     *store.Queries
	clock clock.Clock
	grids *grid.Builder
	dash  *dashboard.Service
	creds *credential.Service
}

// NewHandler cria o handler. Os relatórios reaproveitam as mesmas consultas das telas:
// grades, dashboard, backlog e procurações/certificados.
func NewHandler(pool *pgxpool.Pool, clk clock.Clock, grids *grid.Builder, dash *dashboard.Service, creds *credential.Service) *Handler {
	return &Handler{q: store.New(pool), clock: clk, grids: grids, dash: dash, creds: creds}
}

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/reports/{file}", h.report)
}

// built é um relatório montado, com o sufixo do nome do arquivo (ex.: "2026-10").
type built struct {
	doc  Document
	file string
}

type builder func(h *Handler, ctx context.Context, q url.Values) (built, error)

// Os 6 relatórios iniciais [PRD §36.3].
var reports = map[string]builder{
	"general":     (*Handler).general,
	"pending":     (*Handler).pending,
	"overdue":     (*Handler).overdue,
	"backlog":     (*Handler).backlog,
	"credentials": (*Handler).credentials,
	"module":      (*Handler).module,
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	name, ok := strings.CutSuffix(strings.ToLower(chi.URLParam(r, "file")), ".pdf")
	build, found := reports[name]
	if !ok || !found {
		httpx.WriteError(w, r, apperr.NotFound("REPORT_NOT_FOUND",
			"Relatório não encontrado: use general, pending, overdue, backlog, credentials ou module."))
		return
	}
	b, err := build(h, r.Context(), r.URL.Query())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	b.doc.GeneratedAt = h.clock.Now().In(h.clock.Location())
	data, err := Render(b.doc)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="relatorio-`+name+"-"+b.file+`.pdf"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// --- parâmetros ---

func requiredCompetence(q url.Values) (comp.Competence, error) {
	c, err := comp.Parse(strings.TrimSpace(q.Get("competence")))
	if err != nil {
		return c, validate.Field("competence", "Informe a competência no formato AAAA-MM.")
	}
	return c, nil
}

// backlogParams valida os filtros do backlog/lista com a competência como "competence"
// (lista da competência) ou "before" (backlog das anteriores), e busca todas as linhas.
func backlogParams(q url.Values, c comp.Competence, before bool, state string) (dashboard.BacklogParams, error) {
	v := url.Values{}
	for k, vals := range q {
		v[k] = vals
	}
	v.Del("competence")
	v.Del("before")
	if before {
		v.Set("before", c.String())
	} else {
		v.Set("competence", c.String())
	}
	if state != "" {
		v.Set("state", state)
	}
	v.Del("page")
	v.Del("page_size")
	return dashboard.ParseBacklog(v)
}

// allBacklog percorre as páginas do backlog: o relatório traz todas as linhas.
func (h *Handler) allBacklog(ctx context.Context, p dashboard.BacklogParams) ([]dashboard.BacklogItem, error) {
	const pageSize = 500
	var out []dashboard.BacklogItem
	for page := 1; ; page++ {
		p.Page, p.PageSize = page, pageSize
		b, err := h.dash.Backlog(ctx, p)
		if err != nil {
			return nil, err
		}
		out = append(out, b.Items...)
		if len(b.Items) < pageSize || len(out) >= b.Total {
			return out, nil
		}
	}
}

// --- rótulos e formatação ---

var moduleOrder = []string{"FISCAL", "DECLARATIONS", "PAYROLL", "ACCOUNTING"}

var moduleLabels = map[string]string{
	"FISCAL": "Fiscal", "DECLARATIONS": "Declarações", "PAYROLL": "Folha", "ACCOUNTING": "Contábil", "DCTFWEB": "DCTFWeb",
}

func moduleRank(m string) int {
	for i, v := range moduleOrder {
		if v == m {
			return i
		}
	}
	return len(moduleOrder)
}

var stateLabels = map[string]string{
	"open":        "Em aberto (não finais)",
	"applicable":  "Aplicáveis",
	"done":        "Concluídas",
	"in_progress": "Em andamento",
	"not_started": "Não iniciadas",
	"pending":     "Pendentes",
	"overdue":     "Atrasadas",
}

func pct(v float64) string { return strconv.Itoa(int(v*100+0.5)) + "%" }

func itoa(n int) string { return strconv.Itoa(n) }

func date(d *civil.Date) string {
	if d == nil {
		return "—"
	}
	return d.Format("02/01/2006")
}

func days(n int) string {
	if n <= 0 {
		return "—"
	}
	return itoa(n)
}

// clip limita o texto à largura da coluna (as linhas da tabela têm altura fixa).
func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

func formatCnpj(d string) string {
	if len(d) != 14 {
		return d
	}
	return d[0:2] + "." + d[2:5] + "." + d[5:8] + "/" + d[8:12] + "-" + d[12:14]
}

func responsible(u *dashboard.UserRef) string {
	if u == nil {
		return "—"
	}
	return u.Name
}

// dashboardFilters descreve os filtros do dashboard (grupo, regime e responsável).
func (h *Handler) dashboardFilters(ctx context.Context, f dashboard.Filters) []Field {
	var out []Field
	if f.RegimeGroup != nil {
		label := "Normais"
		if *f.RegimeGroup == company.GroupSimples {
			label = "Simples Nacional"
		}
		out = append(out, Field{"Grupo", label})
	}
	if f.TaxRegime != nil {
		out = append(out, Field{"Regime", company.RegimeLabels[*f.TaxRegime]})
	}
	if f.ResponsibleID != nil {
		name := *f.ResponsibleID
		if u, err := h.q.GetUser(ctx, name); err == nil {
			name = u.Name
		}
		out = append(out, Field{"Responsável", name})
	}
	return out
}

// backlogFilters descreve os filtros do backlog além dos do dashboard.
func (h *Handler) backlogFilters(ctx context.Context, p dashboard.BacklogParams, withState bool) []Field {
	out := h.dashboardFilters(ctx, dashboard.Filters{RegimeGroup: p.RegimeGroup, TaxRegime: p.TaxRegime, ResponsibleID: p.ResponsibleID})
	if p.Module != nil {
		out = append(out, Field{"Módulo", moduleLabels[*p.Module]})
	}
	if withState && p.State != nil {
		out = append(out, Field{"Classe", stateLabels[*p.State]})
	}
	if p.Status != nil {
		out = append(out, Field{"Status", *p.Status})
	}
	if p.OverdueMin != nil {
		out = append(out, Field{"Atraso mínimo", fmt.Sprintf("%d dias", *p.OverdueMin)})
	}
	if p.OverdueMax != nil {
		out = append(out, Field{"Atraso máximo", fmt.Sprintf("%d dias", *p.OverdueMax)})
	}
	if p.Q != nil {
		out = append(out, Field{"Busca", *p.Q})
	}
	return out
}

// moduleCounts conta os itens por módulo, na ordem das telas.
func moduleCounts(items []dashboard.BacklogItem) []Field {
	n := map[string]int{}
	for _, it := range items {
		n[it.Module]++
	}
	out := make([]Field, 0, len(moduleOrder))
	for _, m := range moduleOrder {
		out = append(out, Field{moduleLabels[m], itoa(n[m])})
	}
	return out
}

func distinctCompanies(items []dashboard.BacklogItem) int {
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Company.ID] = true
	}
	return len(seen)
}

// competenceTime é só para o nome dos arquivos sem competência (data de geração).
func competenceTime(t time.Time) string { return t.Format("2006-01-02") }
