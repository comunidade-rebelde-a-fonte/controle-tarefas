import "@fontsource-variable/bricolage-grotesque";
import "@fontsource-variable/inter";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import "./index.css";

const root = document.getElementById("root");
if (!root) throw new Error("elemento #root ausente");

createRoot(root).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
