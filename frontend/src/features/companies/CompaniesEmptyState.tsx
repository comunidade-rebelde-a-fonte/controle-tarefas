import { Building2, Database, FileSpreadsheet, Plus } from "lucide-react";
import { Link } from "react-router";
import { EmptyState } from "@/components/EmptyState";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast-context";
import { isApiError } from "@/lib/api";
import { useLoadDemoData } from "@/services/demo";

/** Sem empresas: cadastrar, importar planilha ou carregar dados fictícios [PRD §49.1]. */
export function CompaniesEmptyState({ onCreate }: { onCreate: () => void }) {
  const toast = useToast();
  const demo = useLoadDemoData();
  return (
    <EmptyState
      icon={Building2}
      title="Nenhuma empresa cadastrada"
      description="Cadastre a primeira empresa, importe a carteira a partir de uma planilha ou carregue os dados fictícios de demonstração para conhecer o sistema."
    >
      <Button onClick={onCreate}>
        <Plus aria-hidden /> Cadastrar empresa
      </Button>
      <Button variant="outline" asChild>
        <Link to="/imports">
          <FileSpreadsheet aria-hidden /> Importar planilha
        </Link>
      </Button>
      <Button
        variant="secondary"
        disabled={demo.isPending}
        onClick={() =>
          demo.mutate(undefined, {
            onSuccess: (res) => toast({ title: "Dados de demonstração carregados", description: `${String(res.companies)} empresas fictícias.`, variant: "success" }),
            onError: (err) =>
              toast({
                title: "Não foi possível carregar a demonstração",
                description: isApiError(err) && err.status === 404 ? "Indisponível neste ambiente." : undefined,
                variant: "error",
              }),
          })
        }
      >
        <Database aria-hidden /> {demo.isPending ? "Carregando…" : "Carregar dados de demonstração"}
      </Button>
    </EmptyState>
  );
}
