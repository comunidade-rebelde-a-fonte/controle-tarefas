import { CircleAlert, CircleCheck, TriangleAlert } from "lucide-react";
import { Link } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { formatDate } from "@/lib/format";
import { type Attention, drilldownHref } from "@/services/dashboard";

type Category = Attention["categories"][number];

// Gravidade de cada categoria: crítica (vermelho) ou alerta (âmbar), sempre com ícone e texto.
const CRITICAL = new Set<Category["key"]>(["OVERDUE", "ERROR", "CERTIFICATE_EXPIRED", "POWER_OF_ATTORNEY_EXPIRED"]);

function CategoryBlock({ category }: { category: Category }) {
  const critical = CRITICAL.has(category.key);
  const Icon = critical ? CircleAlert : TriangleAlert;
  return (
    <li className="grid gap-1.5 border-b py-3 last:border-b-0" aria-label={`${category.label}: ${category.count}`}>
      <div className="flex items-center justify-between gap-2">
        <span className="flex items-center gap-1.5 text-sm font-medium">
          <Icon aria-hidden className={critical ? "size-4 text-destructive" : "size-4 text-warning"} />
          {category.label}
        </span>
        <Badge variant={critical ? "danger" : "warning"}>{category.count}</Badge>
      </div>
      <ul className="grid gap-0.5 pl-5.5 text-xs">
        {category.sample.map((it, i) => (
          <li key={`${it.company.id}:${i}`} className="flex flex-wrap items-center gap-x-1.5">
            {/* Alvo de clique com 24 px de altura (WCAG 2.5.8): sem isso, o espaçamento entre as
                linhas depende da fonte do sistema e o axe acusa target-size fora do macOS. */}
            {it.drilldown ? (
              <Link className="inline-flex min-h-6 items-center font-medium hover:underline" to={drilldownHref(it.drilldown)}>
                {it.company.legal_name}
              </Link>
            ) : (
              <span className="font-medium">{it.company.legal_name}</span>
            )}
            <span className="text-muted-foreground">
              {it.title} · {it.detail}
              {it.date && category.key !== "OVERDUE" ? ` · ${formatDate(it.date)}` : ""}
            </span>
          </li>
        ))}
      </ul>
      {category.drilldown ? (
        <Link className="inline-flex min-h-6 items-center pl-5.5 text-xs text-primary hover:underline" to={drilldownHref(category.drilldown)}>
          Ver todos ({category.count})
        </Link>
      ) : null}
    </li>
  );
}

/**
 * "Exige atenção" [PRD §24.6-§24.7]: as categorias com ocorrências, cada uma com a contagem, uma
 * amostra (5) com link para o item e o "ver todos". Categorias vazias ficam ocultas.
 */
export function AttentionPanel({ attention }: { attention: Attention }) {
  const visible = attention.categories.filter((c) => c.count > 0);
  return (
    <Card aria-label="Exige atenção">
      <CardHeader title="Exige atenção" description="Itens que pedem ação: atrasos, pendências, vencimentos e bloqueios." />
      <CardBody className="py-0">
        {visible.length === 0 ? (
          <p className="flex items-center gap-2 py-4 text-sm text-muted-foreground">
            <CircleCheck aria-hidden className="size-4 text-success" /> Nada exige atenção nesta competência.
          </p>
        ) : (
          <ul aria-label="Categorias de atenção">
            {visible.map((c) => (
              <CategoryBlock key={c.key} category={c} />
            ))}
          </ul>
        )}
      </CardBody>
    </Card>
  );
}
