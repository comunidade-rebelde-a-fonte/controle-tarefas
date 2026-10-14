package pipeline

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"controle/internal/platform/competence"
	"controle/internal/reconciliation/statement"
	"controle/internal/store"
)

// Caminhos determinísticos paralelos ao agente (CC-08): leem o extrato sem LLM e passam pelo
// mesmo portão aritmético; a importação sai com tentativas = 0, como na origem. Cada leitura
// fica no log de extração como a tentativa 0 do "leitor_local".

// localPDF é o atalho processar_local da origem para extratos, agora no job (D-026): o texto
// nativo do PDF no leiaute do cenário (Banco A Fonte, Banco Beta) é lido pelo leitor
// determinístico; fechando o saldo, importa. O porte só usa o leitor com o banco nomeado no texto
// e, se ele não ler ou o saldo não fechar, o documento segue para o triador: na origem, o job caía
// ou o documento ia direto para a revisão sem passar pelo agente (D-027).
func (p *Processor) localPDF(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument, src source, jobID string) (bool, importOutcome, error) {
	if !src.isPDF() || !src.readable() || !src.hasText() {
		return false, importOutcome{}, nil
	}
	text := strings.Join(src.pages, "\n")
	if !statement.IsStatement(text) || !statement.KnownBank(text) {
		return false, importOutcome{}, nil
	}
	q := store.New(tx)
	s, err := statement.FromPDFText(text)
	if err != nil {
		return false, importOutcome{}, p.writeLog(ctx, q, doc, jobID, logEntry{alias: aliasLocalReader, attempt: 0, errText: errText(err)})
	}
	r := statement.Validate(s)
	if err := p.writeLog(ctx, q, doc, jobID, logEntry{alias: aliasLocalReader, attempt: 0, rawOutput: string(jsonBytes(s)), validator: &r}); err != nil {
		return false, importOutcome{}, err
	}
	if !r.OK {
		return false, importOutcome{}, nil
	}
	out, err := p.importStatement(ctx, tx, doc, s, r, 0, "")
	return true, out, err
}

// localCSV é o importador determinístico do CSV de extrato (extrato_de_csv), o caminho paralelo
// que pula o agente e vai direto ao validador. O CSV não traz banco, agência, conta nem período:
// vêm da triagem do documento (ou da revisão humana) e, sem período, do mês da competência do
// envio. Faltando algum, o documento vai para a revisão; CSV ilegível ou que não fecha, também (o
// leitor lê todas as linhas: o agente não teria o que acrescentar).
func (p *Processor) localCSV(ctx context.Context, tx pgx.Tx, doc store.ReconciliationDocument, src source, jobID string) (bool, importOutcome, error) {
	if !src.isText() || !statement.LooksLikeCSV(src.text) {
		return false, importOutcome{}, nil
	}
	q := store.New(tx)
	review := func(reason string, details map[string]any, r *statement.Result) (bool, importOutcome, error) {
		out := importOutcome{status: StatusExtractReview, reason: reason}
		return true, out, p.setState(ctx, q, doc.ID, StatusExtractReview, diagnosis(reason, r, details), nil)
	}
	start, end := doc.PeriodStart, doc.PeriodEnd
	if (start == nil || end == nil) && doc.Competence != nil {
		c := competence.FromDate(*doc.Competence)
		first, last := c.FirstDay(), c.LastDay()
		start, end = &first, &last
	}
	var missing []string
	value := func(field string, v *string) string {
		if v == nil || strings.TrimSpace(*v) == "" {
			missing = append(missing, field)
			return ""
		}
		return *v
	}
	bank, branch, account := value("banco", doc.Bank), value("agencia", doc.Branch), value("conta", doc.Account)
	if start == nil || end == nil {
		missing = append(missing, "periodo")
	}
	if len(missing) > 0 {
		return review(ReasonMissingMetadata, map[string]any{"campos": missing}, nil)
	}
	s, err := statement.FromCSV(src.text, bank, branch, account, dateOnly(*start), dateOnly(*end))
	if err != nil {
		if lerr := p.writeLog(ctx, q, doc, jobID, logEntry{alias: aliasLocalReader, attempt: 0, errText: errText(err)}); lerr != nil {
			return false, importOutcome{}, lerr
		}
		return review(ReasonUnreadableCSV, map[string]any{"erro": errText(err)}, nil)
	}
	r := statement.Validate(s)
	if err := p.writeLog(ctx, q, doc, jobID, logEntry{alias: aliasLocalReader, attempt: 0, rawOutput: string(jsonBytes(s)), validator: &r}); err != nil {
		return false, importOutcome{}, err
	}
	out, err := p.importStatement(ctx, tx, doc, s, r, 0, "")
	return true, out, err
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// errText é o texto do erro do leitor para o log (sem dado do documento além do que o leitor cita).
func errText(err error) string {
	if errors.Is(err, statement.ErrUnreadable) || errors.Is(err, statement.ErrInvalidOutput) {
		return err.Error()
	}
	return "erro de leitura"
}
