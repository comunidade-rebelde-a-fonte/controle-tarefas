import "@testing-library/jest-dom/vitest";
import { cleanup, configure } from "@testing-library/react";
import { afterEach } from "vitest";

// O padrão de 1 s dos findBy*/waitFor estoura sob carga (suíte completa em paralelo com os
// testes de integração do backend); o tempo não altera o resultado dos testes.
configure({ asyncUtilTimeout: 5000 });

afterEach(() => {
  cleanup();
});
