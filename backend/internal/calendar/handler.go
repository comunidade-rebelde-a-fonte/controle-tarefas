package calendar

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"controle/internal/platform/apperr"
	"controle/internal/platform/civil"
	"controle/internal/platform/db"
	"controle/internal/platform/httpx"
	"controle/internal/platform/validate"
	"controle/internal/store"
)

// Calendar é um calendário de feriados (nacional, UF ou município).
type Calendar struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Scope        string  `json:"scope"`
	State        *string `json:"state"`
	City         *string `json:"city"`
	HolidayCount int64   `json:"holiday_count"`
}

// HolidayDTO é um feriado.
type HolidayDTO struct {
	ID           string     `json:"id"`
	CalendarID   string     `json:"calendar_id"`
	CalendarName string     `json:"calendar_name"`
	Date         civil.Date `json:"date"`
	Name         string     `json:"name"`
	Scope        string     `json:"scope"`
}

// HolidayInput cria um feriado configurável [PRD §21.2]. O calendário da UF/município é
// criado sob demanda.
type HolidayInput struct {
	Scope string     `json:"scope" validate:"required,oneof=NATIONAL STATE MUNICIPAL"`
	State *string    `json:"state" validate:"omitempty,uf"`
	City  *string    `json:"city" validate:"omitempty,max=100"`
	Date  civil.Date `json:"date" validate:"required"`
	Name  string     `json:"name" validate:"notblank,max=120"`
}

// DueDateRule é uma regra de vencimento [PRD §21.1, §38.16].
type DueDateRule struct {
	ID                       string  `json:"id"`
	Name                     string  `json:"name"`
	MonthOffset              int32   `json:"month_offset"`
	BaseDay                  int32   `json:"base_day"`
	DayType                  string  `json:"day_type"`
	NonBusinessDayAdjustment string  `json:"non_business_day_adjustment"`
	CalendarID               *string `json:"calendar_id"`
	Notes                    *string `json:"notes"`
	Version                  int32   `json:"version"`
}

// DueDateRuleInput cria ou altera uma regra.
type DueDateRuleInput struct {
	Version                  int32   `json:"version"`
	Name                     string  `json:"name" validate:"notblank,max=120"`
	MonthOffset              int32   `json:"month_offset" validate:"min=0,max=12"`
	BaseDay                  int32   `json:"base_day" validate:"min=1,max=31"`
	DayType                  string  `json:"day_type" validate:"required,oneof=CALENDAR BUSINESS"`
	NonBusinessDayAdjustment string  `json:"non_business_day_adjustment" validate:"required,oneof=PREVIOUS NEXT NONE"`
	CalendarID               *string `json:"calendar_id" validate:"omitempty,uuid"`
	Notes                    *string `json:"notes" validate:"omitempty,max=500"`
}

// Handler expõe /calendars, /holidays e /due-date-rules.
type Handler struct {
	pool *pgxpool.Pool
	q    *store.Queries
}

// NewHandler cria o handler.
func NewHandler(pool *pgxpool.Pool) *Handler { return &Handler{pool: pool, q: store.New(pool)} }

// Routes registra as rotas.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/calendars", h.listCalendars)
	r.Get("/holidays", h.listHolidays)
	r.Post("/holidays", h.createHoliday)
	r.Delete("/holidays/{id}", h.deleteHoliday)
	r.Get("/due-date-rules", h.listRules)
	r.Post("/due-date-rules", h.createRule)
	r.Patch("/due-date-rules/{id}", h.updateRule)
}

func (h *Handler) listCalendars(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListCalendars(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := make([]Calendar, len(rows))
	for i, c := range rows {
		out[i] = Calendar{ID: c.ID, Name: c.Name, Scope: c.Scope, State: c.State, City: c.City, HolidayCount: c.HolidayCount}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) listHolidays(w http.ResponseWriter, r *http.Request) {
	p := store.ListHolidaysParams{}
	if v := r.URL.Query().Get("calendar_id"); v != "" {
		if !httpx.IsUUID(v) {
			httpx.WriteError(w, r, validate.Field("calendar_id", "Identificador inválido."))
			return
		}
		p.CalendarID = &v
	}
	if v := r.URL.Query().Get("year"); v != "" {
		y, err := strconv.ParseInt(v, 10, 32)
		if err != nil || y < 1900 || y > 2200 {
			httpx.WriteError(w, r, validate.Field("year", "Ano inválido."))
			return
		}
		y32 := int32(y)
		p.Year = &y32
	}
	rows, err := h.q.ListHolidays(r.Context(), p)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := make([]HolidayDTO, len(rows))
	for i, x := range rows {
		out[i] = HolidayDTO{ID: x.ID, CalendarID: x.CalendarID, CalendarName: x.CalendarName, Date: civil.Of(x.Date), Name: x.Name, Scope: x.Scope}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func calendarName(scope string, state, city *string) string {
	switch scope {
	case "STATE":
		return "UF " + *state
	case "MUNICIPAL":
		return *city + "/" + *state
	}
	return "Nacional"
}

func (h *Handler) createHoliday(w http.ResponseWriter, r *http.Request) {
	var in HolidayInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if in.State != nil {
		s := strings.ToUpper(strings.TrimSpace(*in.State))
		in.State = &s
	}
	if in.City != nil {
		c := strings.TrimSpace(*in.City)
		in.City = &c
	}
	if err := validate.Struct(in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	switch in.Scope {
	case "NATIONAL":
		in.State, in.City = nil, nil
	case "STATE":
		if in.State == nil {
			httpx.WriteError(w, r, validate.Field("state", "Informe a UF do feriado estadual."))
			return
		}
		in.City = nil
	case "MUNICIPAL":
		if in.State == nil || in.City == nil || *in.City == "" {
			httpx.WriteError(w, r, validate.Field("city", "Informe UF e município do feriado municipal."))
			return
		}
	}
	holidayScope := in.Scope
	if in.Scope == "NATIONAL" {
		holidayScope = "CUSTOM"
	}
	var out HolidayDTO
	err := db.WithTx(r.Context(), h.pool, func(tx pgx.Tx) error {
		q := h.q.WithTx(tx)
		cal, err := q.FindCalendar(r.Context(), store.FindCalendarParams{Scope: in.Scope, State: in.State, City: in.City})
		if errors.Is(err, pgx.ErrNoRows) {
			cal, err = q.InsertCalendar(r.Context(), store.InsertCalendarParams{
				Name: calendarName(in.Scope, in.State, in.City), Scope: in.Scope, State: in.State, City: in.City,
			})
		}
		if err != nil {
			return err
		}
		hol, err := q.InsertHoliday(r.Context(), store.InsertHolidayParams{CalendarID: cal.ID, Date: in.Date.Time, Name: strings.TrimSpace(in.Name), Scope: holidayScope})
		if db.IsConstraint(err, db.UniqueViolation, "holidays_calendar_date_uq") {
			return apperr.Conflict("HOLIDAY_CONFLICT", "Já existe um feriado nesta data para este calendário.")
		}
		if err != nil {
			return err
		}
		out = HolidayDTO{ID: hol.ID, CalendarID: cal.ID, CalendarName: cal.Name, Date: civil.Of(hol.Date), Name: hol.Name, Scope: hol.Scope}
		return nil
	})
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

// deleteHoliday remove um feriado configurável (estadual, municipal ou custom). Feriados
// nacionais do seed não podem ser excluídos.
func (h *Handler) deleteHoliday(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "HOLIDAY_NOT_FOUND", "Feriado não encontrado.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	hol, err := h.q.GetHoliday(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, r, apperr.NotFound("HOLIDAY_NOT_FOUND", "Feriado não encontrado."))
		return
	} else if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if hol.Scope == "NATIONAL" {
		httpx.WriteError(w, r, apperr.Unprocessable("NATIONAL_HOLIDAY", "Feriados nacionais não podem ser excluídos."))
		return
	}
	if err := h.q.DeleteHoliday(r.Context(), id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.NoContent(w)
}

func ruleDTO(x store.DueDateRule) DueDateRule {
	return DueDateRule{ID: x.ID, Name: x.Name, MonthOffset: x.MonthOffset, BaseDay: x.BaseDay, DayType: x.DayType,
		NonBusinessDayAdjustment: x.NonBusinessDayAdjustment, CalendarID: x.CalendarID, Notes: x.Notes, Version: x.Version}
}

func (h *Handler) listRules(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListDueDateRules(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	out := make([]DueDateRule, len(rows))
	for i, x := range rows {
		out[i] = ruleDTO(x)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func mapRuleErr(err error) error {
	switch {
	case db.IsConstraint(err, db.UniqueViolation, "due_date_rules_name_uq"):
		return apperr.Conflict("NAME_CONFLICT", "Já existe uma regra de vencimento com este nome.")
	case db.IsConstraint(err, db.ForeignKeyViolation, ""):
		return validate.Field("calendar_id", "Calendário inexistente.")
	}
	return err
}

func (h *Handler) createRule(w http.ResponseWriter, r *http.Request) {
	var in DueDateRuleInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := validate.Struct(in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	x, err := h.q.InsertDueDateRule(r.Context(), store.InsertDueDateRuleParams{
		Name: strings.TrimSpace(in.Name), MonthOffset: in.MonthOffset, BaseDay: in.BaseDay, DayType: in.DayType,
		NonBusinessDayAdjustment: in.NonBusinessDayAdjustment, CalendarID: in.CalendarID, Notes: in.Notes,
	})
	if err != nil {
		httpx.WriteError(w, r, mapRuleErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, ruleDTO(x))
}

// updateRule altera uma regra. Vale para as próximas gerações; vencimentos já calculados
// não mudam (use a alteração excepcional por instância) [RN-004, §21.3].
func (h *Handler) updateRule(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "id", "DUE_DATE_RULE_NOT_FOUND", "Regra de vencimento não encontrada.")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var in DueDateRuleInput
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := validate.Struct(in); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if in.Version < 1 {
		httpx.WriteError(w, r, validate.Field("version", "Campo obrigatório."))
		return
	}
	if _, err := h.q.GetDueDateRule(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, r, apperr.NotFound("DUE_DATE_RULE_NOT_FOUND", "Regra de vencimento não encontrada."))
		return
	}
	x, err := h.q.UpdateDueDateRule(r.Context(), store.UpdateDueDateRuleParams{
		ID: id, Version: in.Version, Name: strings.TrimSpace(in.Name), MonthOffset: in.MonthOffset, BaseDay: in.BaseDay,
		DayType: in.DayType, NonBusinessDayAdjustment: in.NonBusinessDayAdjustment, CalendarID: in.CalendarID, Notes: in.Notes,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, r, apperr.VersionConflict())
		return
	}
	if err != nil {
		httpx.WriteError(w, r, mapRuleErr(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, ruleDTO(x))
}
