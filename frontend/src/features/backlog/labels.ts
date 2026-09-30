/** Classes da lista de obrigações (drill-down do dashboard), com os mesmos nomes dos KPIs. */
export const STATE_LABELS: Record<string, string> = {
  open: "Em aberto",
  applicable: "Aplicáveis",
  done: "Concluídas",
  in_progress: "Em andamento",
  not_started: "Não iniciadas",
  pending: "Pendentes",
  overdue: "Atrasadas",
};

export const SORT_LABELS: Record<string, string> = {
  days_overdue: "Mais atrasadas primeiro",
  due_date: "Vencimento",
  competence: "Competência",
  company: "Empresa",
};
