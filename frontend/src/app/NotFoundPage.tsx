import { Link } from "react-router";
import { PageHeader } from "@/components/PageHeader";

export function NotFoundPage() {
  return (
    <>
      <PageHeader title="Página não encontrada" />
      <div className="p-6 text-sm">
        <Link className="text-primary underline underline-offset-4" to="/dashboard">
          Voltar ao Dashboard
        </Link>
      </div>
    </>
  );
}
