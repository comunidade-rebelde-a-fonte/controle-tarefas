package langfuse

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Tipos de observação, como na origem: o span raiz é o agente; plan, decide e render são
// chain; parse e tool são tool; a chamada ao LLM é generation.
const (
	TypeAgent      = "agent"
	TypeChain      = "chain"
	TypeTool       = "tool"
	TypeGeneration = "generation"
)

// Níveis de observação do Langfuse.
const (
	LevelDefault = "DEFAULT"
	LevelWarning = "WARNING"
	LevelError   = "ERROR"
)

// ServiceName identifica a aplicação nos traces.
const ServiceName = "controle-integrado"

// Trace é a corrida como vai para o Langfuse: o span raiz (o agente) e os filhos.
type Trace struct {
	ID            string // 32 hex, o mesmo trace_id da trilha local
	RootID        string // 16 hex, estável entre reenvios
	Name          string // nome do agente (nome do trace, como na origem)
	Start, End    time.Time
	Level         string
	StatusMessage string
	Input, Output any
	Tags          []string
	Metadata      map[string]any
	UserID        string
	SessionID     string // a empresa, como na origem
	Spans         []Span
}

// Span é um passo da corrida ou uma chamada ao LLM.
type Span struct {
	ID            string // 16 hex
	ParentID      string // vazio = filho do span raiz
	Name          string
	Type          string
	Start, End    time.Time
	Level         string
	StatusMessage string
	Input, Output any
	Metadata      map[string]any
	// Geração: modelo pedido (provedor/modelo), modelo informado pelo provedor, tokens e custo
	// do gateway (decimal textual).
	Model         string
	ResponseModel string
	TokensIn      *int
	TokensOut     *int
	CostUSD       string
}

// costNumber aceita o custo como número JSON (o gateway informa, por exemplo, 1.25e-05).
var costNumber = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

type otlpValue struct {
	StringValue *string         `json:"stringValue,omitempty"`
	IntValue    *string         `json:"intValue,omitempty"`
	ArrayValue  *otlpArrayValue `json:"arrayValue,omitempty"`
}

type otlpArrayValue struct {
	Values []otlpValue `json:"values"`
}

type otlpAttr struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpStatus struct {
	Code int `json:"code"` // 1 = OK, 2 = ERROR
}

type otlpSpan struct {
	TraceID           string     `json:"traceId"`
	SpanID            string     `json:"spanId"`
	ParentSpanID      string     `json:"parentSpanId,omitempty"`
	Name              string     `json:"name"`
	Kind              int        `json:"kind"`
	StartTimeUnixNano string     `json:"startTimeUnixNano"`
	EndTimeUnixNano   string     `json:"endTimeUnixNano"`
	Attributes        []otlpAttr `json:"attributes"`
	Status            otlpStatus `json:"status"`
}

// attrs monta os atributos já mascarados: todo texto passa por MaskAttribute, como o
// mask_otel_spans da origem.
type attrs struct {
	max  int
	list []otlpAttr
}

func (a *attrs) str(key, v string) *attrs {
	if v == "" {
		return a
	}
	masked := MaskAttribute(v, a.max)
	a.list = append(a.list, otlpAttr{key, otlpValue{StringValue: &masked}})
	return a
}

func (a *attrs) strs(key string, vs []string) *attrs {
	if len(vs) == 0 {
		return a
	}
	values := make([]otlpValue, len(vs))
	for i := range vs {
		values[i] = otlpValue{StringValue: &vs[i]}
	}
	a.list = append(a.list, otlpAttr{key, otlpValue{ArrayValue: &otlpArrayValue{values}}})
	return a
}

// json serializa v e grava como texto (o formato que o SDK Python manda). Nulo ou vazio: nada.
func (a *attrs) json(key string, v any) *attrs {
	if isEmpty(v) {
		return a
	}
	b, err := marshal(v)
	if err != nil {
		return a
	}
	return a.str(key, string(b))
}

func (a *attrs) level(level, message string) *attrs {
	if level != "" && level != LevelDefault {
		a.str("langfuse.observation.level", level)
	}
	return a.str("langfuse.observation.status_message", message)
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case map[string]any:
		return len(x) == 0
	case json.RawMessage:
		s := strings.TrimSpace(string(x))
		return s == "" || s == "null" || s == "{}"
	}
	return false
}

func nanos(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

func status(level string) otlpStatus {
	if level == LevelError {
		return otlpStatus{Code: 2}
	}
	return otlpStatus{Code: 1}
}

// otlp monta o corpo OTLP/HTTP JSON. IDs em hex minúsculo, como manda o mapeamento JSON do OTLP.
func (c *Client) otlp(t Trace) ([]byte, error) {
	root := &attrs{max: c.cfg.MaxPayloadBytes}
	root.str("langfuse.observation.type", TypeAgent).
		str("langfuse.trace.name", t.Name).
		strs("langfuse.trace.tags", t.Tags).
		json("langfuse.trace.metadata", t.Metadata).
		json("langfuse.trace.input", t.Input).
		json("langfuse.trace.output", t.Output).
		json("langfuse.observation.input", t.Input).
		json("langfuse.observation.output", t.Output).
		level(t.Level, t.StatusMessage).
		str("user.id", t.UserID).
		str("session.id", t.SessionID).
		str("langfuse.environment", c.cfg.Environment)
	spans := []otlpSpan{{
		TraceID: t.ID, SpanID: t.RootID, Name: t.Name, Kind: 1,
		StartTimeUnixNano: nanos(t.Start), EndTimeUnixNano: nanos(t.End),
		Attributes: root.list, Status: status(t.Level),
	}}
	for _, s := range t.Spans {
		a := &attrs{max: c.cfg.MaxPayloadBytes}
		a.str("langfuse.observation.type", s.Type).
			json("langfuse.observation.input", s.Input).
			json("langfuse.observation.output", s.Output).
			json("langfuse.observation.metadata", s.Metadata).
			level(s.Level, s.StatusMessage).
			str("langfuse.environment", c.cfg.Environment)
		if s.Type == TypeGeneration {
			provider, name, ok := strings.Cut(s.Model, "/")
			if !ok {
				provider, name = "", s.Model
			}
			a.str("gen_ai.request.model", name).
				str("gen_ai.system", provider).
				str("gen_ai.response.model", s.ResponseModel)
			usage := map[string]any{}
			if s.TokensIn != nil {
				usage["input"] = *s.TokensIn
			}
			if s.TokensOut != nil {
				usage["output"] = *s.TokensOut
			}
			a.json("langfuse.observation.usage_details", usage)
			if costNumber.MatchString(s.CostUSD) {
				a.json("langfuse.observation.cost_details", map[string]any{"total": json.RawMessage(s.CostUSD)})
			}
		}
		parent := s.ParentID
		if parent == "" {
			parent = t.RootID
		}
		spans = append(spans, otlpSpan{
			TraceID: t.ID, SpanID: s.ID, ParentSpanID: parent, Name: s.Name, Kind: 1,
			StartTimeUnixNano: nanos(s.Start), EndTimeUnixNano: nanos(s.End),
			Attributes: a.list, Status: status(s.Level),
		})
	}
	service := ServiceName
	return marshal(map[string]any{"resourceSpans": []any{map[string]any{
		"resource":   map[string]any{"attributes": []otlpAttr{{"service.name", otlpValue{StringValue: &service}}}},
		"scopeSpans": []any{map[string]any{"scope": map[string]any{"name": ServiceName}, "spans": spans}},
	}}})
}
