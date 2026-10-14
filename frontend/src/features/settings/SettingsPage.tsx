import { useSearchParams } from "react-router";
import { PageHeader } from "@/components/PageHeader";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AIModelsTab } from "./AIModelsTab";
import { CatalogTab } from "./CatalogTab";
import { DueDateRulesTab } from "./DueDateRulesTab";
import { HolidaysTab } from "./HolidaysTab";
import { ThresholdsTab } from "./ThresholdsTab";
import { UsersTab } from "./UsersTab";

const TABS = [
  ["catalog", "Catálogo de obrigações"],
  ["due-dates", "Regras de vencimento"],
  ["holidays", "Feriados"],
  ["thresholds", "Faixas de validade"],
  ["users", "Responsáveis"],
  ["ai", "IA"],
] as const;

/** Cadastros e Configurações [PRD §6 item 10, §20, §21, §19.3; IA: 08 §9]. */
export function SettingsPage() {
  const [params, setParams] = useSearchParams();
  const tab = TABS.some(([t]) => t === params.get("tab")) ? (params.get("tab") ?? "catalog") : "catalog";
  return (
    <>
      <PageHeader title="Cadastros e Configurações" description="Catálogo de obrigações, vencimentos, feriados, faixas de validade, responsáveis e modelos de IA." />
      <Tabs
        value={tab}
        onValueChange={(v) => setParams(v === "catalog" ? {} : { tab: v }, { replace: true })}
      >
        <TabsList aria-label="Seções de configuração">
          {TABS.map(([value, label]) => (
            <TabsTrigger key={value} value={value}>
              {label}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="catalog">
          <CatalogTab />
        </TabsContent>
        <TabsContent value="due-dates">
          <DueDateRulesTab />
        </TabsContent>
        <TabsContent value="holidays">
          <HolidaysTab today={new Date()} />
        </TabsContent>
        <TabsContent value="thresholds">
          <ThresholdsTab />
        </TabsContent>
        <TabsContent value="users">
          <UsersTab />
        </TabsContent>
        <TabsContent value="ai">
          <AIModelsTab />
        </TabsContent>
      </Tabs>
    </>
  );
}
