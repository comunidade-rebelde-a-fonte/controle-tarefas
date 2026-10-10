import { createBrowserRouter, Navigate } from "react-router";
import { DashboardPage } from "@/features/dashboard/DashboardPage";
import { CompaniesPage } from "@/features/companies/CompaniesPage";
import { FiscalPage } from "@/features/fiscal/FiscalPage";
import { DeclarationsPage } from "@/features/declarations/DeclarationsPage";
import { AccountingPage } from "@/features/accounting/AccountingPage";
import { PayrollPage } from "@/features/payroll/PayrollPage";
import { DctfwebPage } from "@/features/dctfweb/DctfwebPage";
import { CredentialsPage } from "@/features/credentials/CredentialsPage";
import { BacklogPage } from "@/features/backlog/BacklogPage";
import { ObligationsListPage } from "@/features/obligations/ObligationsListPage";
import { SettingsPage } from "@/features/settings/SettingsPage";
import { ImportsPage } from "@/features/imports/ImportsPage";
import { ReportsPage } from "@/features/reports/ReportsPage";
import { CompanyDetailPage } from "@/features/companies/detail/CompanyDetailPage";
import { FactorRPortfolioPage } from "@/features/factor-r/FactorRPortfolioPage";
import { FactorRLayout } from "@/features/factor-r/FactorRLayout";
import { AgentsPage } from "@/features/factor-r/agents/AgentsPage";
import { InboxPage } from "@/features/factor-r/inbox/InboxPage";
import { PgdasDocumentPage } from "@/features/factor-r/inbox/PgdasDocumentPage";
import { ObservabilityPage } from "@/features/factor-r/runs/ObservabilityPage";
import { RunDetailPage } from "@/features/factor-r/runs/RunDetailPage";
import { DocumentPage } from "@/features/reconciliation/documents/DocumentPage";
import { DocumentsPage } from "@/features/reconciliation/documents/DocumentsPage";
import { ReconciliationLayout } from "@/features/reconciliation/ReconciliationLayout";
import { ReviewQueuePage } from "@/features/reconciliation/review/ReviewQueuePage";
import { Layout } from "./Layout";
import { NotFoundPage } from "./NotFoundPage";

export const routes = [
  {
    path: "/",
    element: <Layout />,
    children: [
      { index: true, element: <Navigate to="/dashboard" replace /> },
      { path: "dashboard", element: <DashboardPage /> },
      { path: "companies", element: <CompaniesPage /> },
      { path: "companies/:id", element: <CompanyDetailPage /> },
      { path: "fiscal", element: <FiscalPage /> },
      { path: "declarations", element: <DeclarationsPage /> },
      { path: "accounting", element: <AccountingPage /> },
      { path: "payroll", element: <PayrollPage /> },
      { path: "dctfweb", element: <DctfwebPage /> },
      {
        // Área Fator R com submenu (D-025): carteira, inbox PGDAS-D, agentes e observabilidade.
        path: "factor-r",
        element: <FactorRLayout />,
        children: [
          { index: true, element: <FactorRPortfolioPage /> },
          { path: "inbox", element: <InboxPage /> },
          { path: "inbox/:id", element: <PgdasDocumentPage /> },
          { path: "agents", element: <AgentsPage /> },
          { path: "runs", element: <ObservabilityPage /> },
          { path: "runs/:traceId", element: <RunDetailPage /> },
        ],
      },
      {
        // Área Conciliação com submenu (TASK-021a, D-032): fila de revisão e documentos.
        path: "reconciliation",
        element: <ReconciliationLayout />,
        children: [
          { index: true, element: <ReviewQueuePage /> },
          { path: "documents", element: <DocumentsPage /> },
          { path: "documents/:id", element: <DocumentPage /> },
        ],
      },
      { path: "credentials", element: <CredentialsPage /> },
      { path: "backlog", element: <BacklogPage /> },
      { path: "obligations", element: <ObligationsListPage /> },
      { path: "settings", element: <SettingsPage /> },
      { path: "imports", element: <ImportsPage /> },
      { path: "reports", element: <ReportsPage /> },
      { path: "*", element: <NotFoundPage /> },
    ],
  },
];

export const router = createBrowserRouter(routes);
