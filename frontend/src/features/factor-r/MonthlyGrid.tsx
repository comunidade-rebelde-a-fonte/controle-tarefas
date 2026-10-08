import { useState } from "react";
import { ErrorAlert } from "@/components/ErrorAlert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, Td, Th } from "@/components/ui/table";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { addMonths, displayCompetence } from "@/lib/competence";
import { formatMoney, moneyToInput, parseMoneyInput } from "@/lib/decimal";
import { type FactorRMovement, useFactorRMovements, usePutFactorRMovement } from "@/services/factorR";
import { ORIGIN_LABELS } from "./labels";

const FIELDS = [
  ["receita_bruta", "Receita bruta"],
  ["pro_labore", "Pró-labore"],
  ["salarios_13_ferias", "Salários + 13º/férias"],
  ["cpp_recolhida", "CPP recolhida"],
  ["fgts_recolhido", "FGTS recolhido"],
] as const;

type FieldName = (typeof FIELDS)[number][0];
type Values = Record<FieldName, string>;

function initialValues(m: FactorRMovement | undefined, detailing: boolean): Values {
  const v = Object.fromEntries(FIELDS.map(([f]) => [f, m ? moneyToInput(m[f]) : ""])) as Values;
  // "Detalhar folha" parte do total declarado no PGDAS-D em salários, como na origem.
  if (m && detailing && m.folha_total_declarada_pgdas !== null) v.salarios_13_ferias = moneyToInput(m.folha_total_declarada_pgdas);
  return v;
}

type RowState = { kind: "idle" } | { kind: "error"; message: string };

function MonthRow({ companyId, competence, movement }: { companyId: string; competence: string; movement: FactorRMovement | undefined }) {
  const put = usePutFactorRMovement(companyId);
  const toast = useToast();
  const declared = movement?.origem === "pgdas" ? movement.folha_total_declarada_pgdas : null;
  const [detailing, setDetailing] = useState(false);
  const [values, setValues] = useState<Values>(() => initialValues(movement, false));
  const [note, setNote] = useState(movement?.observacao ?? "");
  const [state, setState] = useState<RowState>({ kind: "idle" });
  const label = displayCompetence(competence);
  const showDeclared = declared !== null && !detailing;

  async function save() {
    const body: Record<string, string> = {};
    for (const [field, fieldLabel] of FIELDS) {
      if (showDeclared && field !== "receita_bruta") continue;
      const parsed = parseMoneyInput(values[field]);
      if (parsed === null) {
        setState({ kind: "error", message: `${fieldLabel}: valor inválido (use 1.234,56)` });
        return;
      }
      body[field] = parsed;
    }
    if (showDeclared) {
      // Salvar sem detalhar mantém a folha declarada como salários (a API grava a divisão).
      body.salarios_13_ferias = declared;
    }
    // mutateAsync, e não os callbacks do mutate: ao gravar, a grade recarrega e esta linha é
    // remontada com a nova versão do mês, e callbacks de um componente desmontado não rodam.
    try {
      await put.mutateAsync({ competence, body: { ...body, version: movement?.version ?? 0, observacao: note.trim() === "" ? null : note } });
      toast({ title: `Movimento de ${label} salvo`, variant: "success" });
    } catch (err) {
      const message = isApiError(err) ? err.message : "Não foi possível salvar.";
      setState({ kind: "error", message });
      toast({ title: `Movimento de ${label} não foi salvo`, description: message, variant: "error" });
    }
  }

  return (
    <tr data-competencia={competence}>
      <Th scope="row" className="static bg-card font-medium whitespace-nowrap text-foreground">
        {label}
      </Th>
      <Td>{movement ? <Badge variant={movement.origem === "pgdas" ? "default" : "muted"}>{ORIGIN_LABELS[movement.origem]}</Badge> : <span className="text-xs text-muted-foreground">sem lançamento</span>}</Td>
      {FIELDS.map(([field, fieldLabel]) =>
        showDeclared && field !== "receita_bruta" ? null : (
          <Td key={field}>
            <Input
              aria-label={`${fieldLabel} ${label}`}
              inputMode="decimal"
              value={values[field]}
              onChange={(e) => {
                setValues({ ...values, [field]: e.target.value });
                setState({ kind: "idle" });
              }}
              className="h-8 w-28 text-right tabular-nums"
            />
          </Td>
        ),
      )}
      {showDeclared ? (
        <Td colSpan={4} data-testid={`folha-pgdas-${competence}`} className="text-sm">
          Folha declarada no PGDAS-D: <strong className="tabular-nums">{formatMoney(declared)}</strong>
          <span className="block text-xs text-muted-foreground">O extrato não separa pró-labore, salários, CPP e FGTS.</span>
          <Button
            variant="link"
            size="sm"
            className="h-6 px-0"
            onClick={() => {
              setDetailing(true);
              setValues(initialValues(movement, true));
            }}
          >
            Detalhar folha
          </Button>
        </Td>
      ) : null}
      <Td className="text-right whitespace-nowrap tabular-nums" data-testid={`folha-${competence}`}>
        {movement ? formatMoney(movement.folha_mes) : "—"}
      </Td>
      <Td>
        <Input aria-label={`Observação ${label}`} value={note} onChange={(e) => setNote(e.target.value)} className="h-8 w-40" />
      </Td>
      <Td className="whitespace-nowrap">
        <Button variant="outline" size="sm" onClick={() => void save()} disabled={put.isPending} aria-label={`Salvar ${label}`}>
          {put.isPending ? "Salvando…" : "Salvar"}
        </Button>
        {state.kind === "error" ? (
          <span role="alert" className="ml-2 text-xs text-red-800">
            {state.message}
          </span>
        ) : null}
      </Td>
    </tr>
  );
}

/**
 * Grade mensal da janela do PA [REQ-009, FR-03]: receita e folha de cada um dos 12 meses,
 * editáveis à mão. Cada linha grava a competência inteira (origem manual), e o cálculo acima
 * é atualizado. Mês do PGDAS-D mostra só a folha total declarada, até "Detalhar folha".
 */
export function MonthlyGrid({ companyId, pa }: { companyId: string; pa: string }) {
  const from = addMonths(pa, -12);
  const to = addMonths(pa, -1);
  const movements = useFactorRMovements(companyId, from, to);
  const months = Array.from({ length: 12 }, (_, i) => addMonths(from, i));
  const byMonth = new Map((movements.data?.items ?? []).map((m) => [m.competence, m]));

  return (
    <Card>
      <CardHeader
        title="Movimentos mensais"
        description={`Janela do PA ${displayCompetence(pa)}: ${displayCompetence(from)} a ${displayCompetence(to)}. Valores em reais; a competência é o mês da receita ou da folha.`}
      />
      <CardBody className="p-0">
        <ErrorAlert error={movements.error} fallback="Não foi possível carregar os movimentos." />
        {movements.isPending ? (
          <Skeleton className="m-4 h-64" />
        ) : (
          <div className="overflow-x-auto">
            <Table aria-label={`Movimentos de ${displayCompetence(from)} a ${displayCompetence(to)}`}>
              <thead>
                <tr>
                  <Th scope="col">Competência</Th>
                  <Th scope="col">Origem</Th>
                  {FIELDS.map(([f, l]) => (
                    <Th key={f} scope="col">
                      {l}
                    </Th>
                  ))}
                  <Th scope="col" className="text-right">
                    Folha do mês
                  </Th>
                  <Th scope="col">Observação</Th>
                  <Th scope="col">
                    <span className="sr-only">Ações</span>
                  </Th>
                </tr>
              </thead>
              <tbody>
                {months.map((m) => (
                  <MonthRow key={`${m}-${String(byMonth.get(m)?.version ?? 0)}`} companyId={companyId} competence={m} movement={byMonth.get(m)} />
                ))}
              </tbody>
            </Table>
          </div>
        )}
      </CardBody>
    </Card>
  );
}
