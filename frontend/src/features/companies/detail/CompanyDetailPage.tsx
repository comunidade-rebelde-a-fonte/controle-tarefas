import { useParams } from "react-router";
import { PlaceholderPage } from "@/components/PlaceholderPage";

export function CompanyDetailPage() {
  const { id } = useParams();
  return <PlaceholderPage title="Ficha da empresa" description={`Empresa ${id ?? ""}`} availableFrom="T-M1-15" />;
}
