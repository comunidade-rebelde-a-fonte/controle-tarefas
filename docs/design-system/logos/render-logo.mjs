// Gera o logo do menu a partir do SVG oficial (TASK-029, D-043). O SVG oficial embute imagens
// (~1,7 MB) e tem um retângulo preto opaco atrás do texto: o Chromium do Playwright (o do E2E)
// renderiza o SVG, o preto vira transparência (alfa = maior canal; sobre preto, a aparência é a
// mesma), a margem transparente é recortada e o PNG sai na largura pedida. Da raiz do repositório:
//
//   node docs/design-system/logos/render-logo.mjs e2e/package.json
//     docs/design-system/logos/inovasala-by-ceath-branco.svg
//     frontend/src/assets/brand/inovasala-by-ceath-branco.png 400 --preto-transparente
//
// (os cinco argumentos na mesma linha).
import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const [, , e2ePackage, svgPath, outPath, widthArg, flag] = process.argv;
const require = createRequire(resolve(e2ePackage));
const { chromium } = require("@playwright/test");

const size = 2000;
const svg = readFileSync(svgPath).toString("base64");
const browser = await chromium.launch();
try {
  const page = await browser.newPage({ viewport: { width: size, height: size } });
  await page.setContent(
    `<html><body style="margin:0;background:transparent"><img id="logo" width="${size}" height="${size}" src="data:image/svg+xml;base64,${svg}"></body></html>`,
  );
  await page.waitForFunction(() => {
    const img = document.getElementById("logo");
    return img.complete && img.naturalWidth > 0;
  });
  const result = await page.evaluate(
    ({ size, width, keyBlack }) => {
      const img = document.getElementById("logo");
      const full = document.createElement("canvas");
      full.width = size;
      full.height = size;
      const ctx = full.getContext("2d");
      ctx.drawImage(img, 0, 0, size, size);
      const pixels = ctx.getImageData(0, 0, size, size);
      const data = pixels.data;
      if (keyBlack) {
        // Preto vira transparência (arte desenhada sobre preto opaco): alfa = maior canal; cor
        // dividida pelo alfa. Sobre preto, o resultado é o mesmo do original.
        for (let i = 0; i < data.length; i += 4) {
          const a = data[i + 3] / 255;
          const m = Math.max(data[i], data[i + 1], data[i + 2]);
          if (m === 0) {
            data[i + 3] = 0;
            continue;
          }
          const k = 255 / m;
          data[i] = Math.min(255, Math.round(data[i] * k));
          data[i + 1] = Math.min(255, Math.round(data[i + 1] * k));
          data[i + 2] = Math.min(255, Math.round(data[i + 2] * k));
          data[i + 3] = Math.round(m * a);
        }
        ctx.putImageData(pixels, 0, 0);
      }
      let minX = size, minY = size, maxX = -1, maxY = -1;
      for (let y = 0; y < size; y++) {
        for (let x = 0; x < size; x++) {
          if (data[(y * size + x) * 4 + 3] > 8) {
            if (x < minX) minX = x;
            if (y < minY) minY = y;
            if (x > maxX) maxX = x;
            if (y > maxY) maxY = y;
          }
        }
      }
      const w = maxX - minX + 1;
      const h = maxY - minY + 1;
      const out = document.createElement("canvas");
      out.width = width;
      out.height = Math.round((h * width) / w);
      const octx = out.getContext("2d");
      octx.imageSmoothingQuality = "high";
      octx.drawImage(full, minX, minY, w, h, 0, 0, out.width, out.height);
      return { url: out.toDataURL("image/png"), box: [minX, minY, maxX, maxY], height: out.height };
    },
    { size, width: Number(widthArg), keyBlack: flag === "--preto-transparente" },
  );
  writeFileSync(outPath, Buffer.from(result.url.split(",")[1], "base64"));
  console.log("recorte", result.box, "->", outPath, `${widthArg}x${result.height}`);
} finally {
  await browser.close();
}
