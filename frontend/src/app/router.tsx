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
      { path: "factor-r", element: <FactorRPortfolioPage /> },
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
