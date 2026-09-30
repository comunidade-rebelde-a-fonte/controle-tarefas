package competence

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	comp "controle/internal/platform/competence"
	"controle/internal/platform/httpx"
	"controle/internal/store"
)

// CompetenceInfo descreve uma competência gerada.
type CompetenceInfo struct {
	Competence    comp.Competence `json:"competence"`
	CreatedAt     time.Time       `json:"created_at"`
	Companies     int64           `json:"companies"`
	Instances     int64           `json:"instances"`
	OpenPeriods   int64           `json:"open_periods"`
	ClosedPeriods int64           `json:"closed_periods"`
}

// Latest é a resposta de GET /competencies/latest.
type Latest struct {
	Competence comp.Competence `json:"competence"`
	Current    comp.Competence `json:"current"`
}

// Handler expõe /competencies.
type Handler struct {
	engine *Engine
	q      *store.Queries
}

// NewHandler cria o handler.
func NewHandler(pool *pgxpool.Pool, e *Engine) *Handler {
	return &Handler{engine: e, q: store.New(pool)}
}

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/competencies", h.list)
	r.Get("/competencies/latest", h.latest)
	r.Post("/competencies/ensure-current", h.ensureCurrent)
}

// EnsureIfCurrent dispara a geração quando a competência pedida é a corrente e ainda não
// existe (terceiro gatilho do PRD §10.2). Handlers de grade chamam antes de consultar.
func (e *Engine) EnsureIfCurrent(ctx context.Context, c comp.Competence) error {
	if !c.Equal(e.Current()) {
		return nil
	}
	exists, err := store.New(e.pool).CompetenceExists(ctx, c.String())
	if err != nil || exists {
		return err
	}
	_, err = e.Ensure(ctx, c)
	return err
}

// LatestGenerated devolve a última competência gerada (nil se nenhuma).
func (e *Engine) LatestGenerated(ctx context.Context) (*comp.Competence, error) {
	key, err := store.New(e.pool).LatestCompetence(ctx)
	if err != nil {
		if isNoRows(err) {
			return nil, nil
		}
		return nil, err
	}
	c, err := comp.Parse(key)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if err := h.engine.EnsureIfCurrent(r.Context(), h.engine.Current()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	rows, err := h.q.ListCompetenciesWithCounts(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := make([]CompetenceInfo, 0, len(rows))
	for _, row := range rows {
		c, err := comp.Parse(row.CompetenceKey)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, CompetenceInfo{Competence: c, CreatedAt: row.CreatedAt, Companies: row.Companies, Instances: row.Instances,
			OpenPeriods: row.OpenPeriods, ClosedPeriods: row.ClosedPeriods})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

// latest devolve a competência mais recente (a corrente é garantida antes) [PRD §24.1].
func (h *Handler) latest(w http.ResponseWriter, r *http.Request) {
	cur := h.engine.Current()
	if err := h.engine.EnsureIfCurrent(r.Context(), cur); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	latest, err := h.engine.LatestGenerated(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := Latest{Competence: cur, Current: cur}
	if latest != nil && latest.After(cur) {
		out.Competence = *latest
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) ensureCurrent(w http.ResponseWriter, r *http.Request) {
	s, err := h.engine.EnsureCurrent(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}
